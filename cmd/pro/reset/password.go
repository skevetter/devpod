package reset

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	storagev1 "github.com/loft-sh/api/v4/pkg/apis/storage/v1"
	"github.com/skevetter/devpod/cmd/pro/flags"
	"github.com/skevetter/devpod/pkg/platform/kube"
	"github.com/skevetter/devpod/pkg/random"
	"github.com/skevetter/log"
	"github.com/skevetter/log/survey"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// PasswordCmd holds the flags.
type PasswordCmd struct {
	*flags.GlobalFlags

	User     string
	Password string
	Create   bool
	Force    bool

	Log log.Logger
}

// NewPasswordCmd creates a new command.
func NewPasswordCmd(globalFlags *flags.GlobalFlags) *cobra.Command {
	cmd := &PasswordCmd{
		GlobalFlags: globalFlags,
		Log:         log.GetInstance(),
	}
	description := `
Resets the password of a user.

Example:
devpod pro reset password
devpod pro reset password --user admin
#######################################################
	`
	c := &cobra.Command{
		Use:   "password",
		Short: "Resets the password of a user",
		Long:  description,
		Args:  cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			return cmd.Run(cobraCmd.Context())
		},
	}

	c.Flags().StringVar(&cmd.User, "user", "admin", "The name of the user to reset the password")
	c.Flags().StringVar(&cmd.Password, "password", "", "The new password to use")
	c.Flags().BoolVar(&cmd.Create, "create", false, "Creates the user if it does not exist")
	c.Flags().BoolVar(&cmd.Force, "force", false, "If user had no password will create one")
	return c
}

// Run executes the functionality.
func (cmd *PasswordCmd) Run(ctx context.Context) error {
	restConfig, err := ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("get kube config: %w", err)
	}

	managementClient, err := kube.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	cmd.Log.Infof("Resetting password of user %s", cmd.User)
	user, err := cmd.resolveUser(ctx, managementClient)
	if err != nil {
		return err
	}

	// Compute desired PasswordRef in-memory (no API call)
	refChanged, err := cmd.ensurePasswordRef(user)
	if err != nil {
		return err
	}

	password, err := cmd.resolvePassword()
	if err != nil {
		return err
	}

	// Write secret before persisting the user's PasswordRef
	passwordHash := fmt.Appendf(nil, "%x", sha256.Sum256([]byte(password)))
	if err := cmd.upsertPasswordSecret(ctx, managementClient, user, passwordHash); err != nil {
		return err
	}

	// Now persist the user's PasswordRef if it was changed
	if err := cmd.persistPasswordRef(ctx, managementClient, user, refChanged); err != nil {
		return err
	}

	cmd.Log.Done("reset user password")
	return nil
}

func (cmd *PasswordCmd) resolveUser(
	ctx context.Context,
	managementClient kube.Interface,
) (*storagev1.User, error) {
	user, err := managementClient.Loft().
		StorageV1().
		Users().
		Get(ctx, cmd.User, metav1.GetOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		return nil, fmt.Errorf("get user: %w", err)
	} else if kerrors.IsNotFound(err) {
		if !cmd.Create {
			return nil, fmt.Errorf(
				"user %s was not found, run with '--create' to create this user automatically",
				cmd.User,
			)
		}

		user, err = managementClient.Loft().
			StorageV1().
			Users().
			Create(ctx, &storagev1.User{
				ObjectMeta: metav1.ObjectMeta{
					Name: cmd.User,
				},
				Spec: storagev1.UserSpec{
					Username: cmd.User,
					Subject:  cmd.User,
					Groups: []string{
						"system:masters",
					},
				},
			}, metav1.CreateOptions{})
		if err != nil {
			return nil, err
		}
	}

	return user, nil
}

// persistPasswordRef updates the user resource if the PasswordRef was changed.
func (cmd *PasswordCmd) persistPasswordRef(
	ctx context.Context,
	managementClient kube.Interface,
	user *storagev1.User,
	changed bool,
) error {
	if !changed {
		return nil
	}
	_, err := managementClient.Loft().
		StorageV1().
		Users().
		Update(ctx, user, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}

func (cmd *PasswordCmd) ensurePasswordRef(user *storagev1.User) (bool, error) {
	if hasCompletePasswordRef(user) {
		return false, nil
	}

	if err := cmd.fillPasswordRef(user); err != nil {
		return false, err
	}

	return true, nil
}

func hasCompletePasswordRef(user *storagev1.User) bool {
	ref := user.Spec.PasswordRef
	return ref != nil && ref.SecretName != "" && ref.SecretNamespace != "" && ref.Key != ""
}

func (cmd *PasswordCmd) fillPasswordRef(user *storagev1.User) error {
	ref := user.Spec.PasswordRef
	// If PasswordRef exists with name and namespace but missing key, just default the key
	if ref != nil && ref.SecretName != "" && ref.SecretNamespace != "" {
		ref.Key = "password"
		return nil
	}

	if !cmd.Force {
		return fmt.Errorf(
			"user %s had no password. If you want to force password creation, please run with the '--force' flag",
			cmd.User,
		)
	}

	user.Spec.PasswordRef = defaultPasswordRef(user.Spec.PasswordRef)
	return nil
}

func defaultPasswordRef(ref *storagev1.SecretRef) *storagev1.SecretRef {
	if ref == nil {
		ref = &storagev1.SecretRef{}
	}
	if ref.SecretName == "" {
		ref.SecretName = "loft-password-" + random.String(5)
	}
	if ref.SecretNamespace == "" {
		ref.SecretNamespace = "loft"
	}
	if ref.Key == "" {
		ref.Key = "password"
	}
	return ref
}

func (cmd *PasswordCmd) resolvePassword() (string, error) {
	if cmd.Password != "" {
		return cmd.Password, nil
	}

	for {
		password, err := cmd.Log.Question(&survey.QuestionOptions{
			Question:   "Please enter a new password",
			IsPassword: true,
		})
		if err != nil {
			return "", err
		}

		password = strings.TrimSpace(password)
		if password != "" {
			return password, nil
		}

		cmd.Log.Error("Please enter a password")
	}
}

func (cmd *PasswordCmd) upsertPasswordSecret(
	ctx context.Context,
	managementClient kube.Interface,
	user *storagev1.User,
	passwordHash []byte,
) error {
	ref := user.Spec.PasswordRef
	passwordSecret, err := managementClient.CoreV1().
		Secrets(ref.SecretNamespace).
		Get(ctx, ref.SecretName, metav1.GetOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		return fmt.Errorf("get password secret: %w", err)
	}

	if kerrors.IsNotFound(err) {
		_, err = managementClient.CoreV1().
			Secrets(ref.SecretNamespace).
			Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ref.SecretName,
					Namespace: ref.SecretNamespace,
				},
				Data: map[string][]byte{
					ref.Key: passwordHash,
				},
			}, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create password secret: %w", err)
		}
		return nil
	}

	if passwordSecret.Data == nil {
		passwordSecret.Data = map[string][]byte{}
	}
	passwordSecret.Data[ref.Key] = passwordHash
	_, err = managementClient.CoreV1().
		Secrets(ref.SecretNamespace).
		Update(ctx, passwordSecret, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update password secret: %w", err)
	}

	return nil
}
