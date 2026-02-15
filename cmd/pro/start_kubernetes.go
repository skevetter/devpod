package pro

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	netUrl "net/url"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/denisbrodbeck/machineid"
	jsonpatch "github.com/evanphx/json-patch"
	storagev1 "github.com/loft-sh/api/v4/pkg/apis/storage/v1"
	loftclientset "github.com/loft-sh/api/v4/pkg/clientset/versioned"
	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/util"
	"github.com/skevetter/log"
	"github.com/skevetter/log/survey"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset"
)

func uninstall(ctx context.Context, kubeClient kubernetes.Interface, restConfig *rest.Config, kubeContext, namespace string, log log.Logger) error {
	releaseName := "devpod-pro"
	deploy, err := kubeClient.AppsV1().Deployments(namespace).Get(ctx, defaultDeploymentName, metav1.GetOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		return err
	} else if deploy != nil && deploy.Labels != nil && deploy.Labels["release"] != "" {
		releaseName = deploy.Labels["release"]
	}

	args := []string{
		"uninstall",
		releaseName,
		"--kube-context",
		kubeContext,
		"--namespace",
		namespace,
	}
	log.Infof("Executing command: helm %s", strings.Join(args, " "))
	output, err := exec.Command("helm", args...).CombinedOutput()
	if err != nil {
		log.Errorf("error during helm command: %s (%v)", string(output), err)
	}

	// we also cleanup the validating webhook configuration and apiservice
	apiRegistrationClient, err := clientset.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	err = apiRegistrationClient.ApiregistrationV1().APIServices().Delete(ctx, "v1.management.loft.sh", metav1.DeleteOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		return err
	}

	err = deleteUser(ctx, restConfig, "admin")
	if err != nil {
		return err
	}

	err = kubeClient.CoreV1().Secrets(namespace).Delete(context.Background(), "loft-user-secret-admin", metav1.DeleteOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		return err
	}

	err = kubeClient.CoreV1().Secrets(namespace).Delete(context.Background(), LoftRouterDomainSecret, metav1.DeleteOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		return err
	}

	// we also cleanup the validating webhook configuration and apiservice
	err = kubeClient.AdmissionregistrationV1().ValidatingWebhookConfigurations().Delete(ctx, "loft-agent", metav1.DeleteOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		return err
	}

	err = apiRegistrationClient.ApiregistrationV1().APIServices().Delete(ctx, "v1alpha1.tenancy.kiosk.sh", metav1.DeleteOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		return err
	}

	err = apiRegistrationClient.ApiregistrationV1().APIServices().Delete(ctx, "v1.cluster.loft.sh", metav1.DeleteOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		return err
	}

	err = kubeClient.CoreV1().ConfigMaps(namespace).Delete(ctx, "loft-agent-controller", metav1.DeleteOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		return err
	}

	err = kubeClient.CoreV1().ConfigMaps(namespace).Delete(ctx, "loft-applied-defaults", metav1.DeleteOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		return err
	}

	log.WriteString(logrus.InfoLevel, "\n")
	log.Done("uninstalled DevPod Pro")
	log.WriteString(logrus.InfoLevel, "\n")

	return nil
}

func isAlreadyInstalled(ctx context.Context, kubeClient kubernetes.Interface, namespace string) (bool, error) {
	_, err := kubeClient.AppsV1().Deployments(namespace).Get(ctx, defaultDeploymentName, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			return false, nil
		}

		return false, fmt.Errorf("error accessing kubernetes cluster: %w", err)
	}

	return true, nil
}

func getDefaultPassword(ctx context.Context, kubeClient kubernetes.Interface, namespace string) (string, error) {
	loftNamespace, err := kubeClient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			loftNamespace, err := kubeClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}, metav1.CreateOptions{})
			if err != nil {
				return "", err
			}

			return string(loftNamespace.UID), nil
		}

		return "", err
	}

	return string(loftNamespace.UID), nil
}

func isInstalledLocally(ctx context.Context, kubeClient kubernetes.Interface, namespace string) bool {
	_, err := kubeClient.NetworkingV1().Ingresses(namespace).Get(ctx, "loft-ingress", metav1.GetOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		_, err = kubeClient.NetworkingV1beta1().Ingresses(namespace).Get(ctx, "loft-ingress", metav1.GetOptions{})
		return kerrors.IsNotFound(err)
	}

	return kerrors.IsNotFound(err)
}

func enterHostNameQuestion(log log.Logger) (string, error) {
	return log.Question(&survey.QuestionOptions{
		Question: fmt.Sprintf("Enter a hostname for your %s instance (e.g. loft.my-domain.tld): \n ", "DevPod Pro"),
		ValidationFunc: func(answer string) error {
			u, err := netUrl.Parse("https://" + answer)
			if err != nil || u.Path != "" || u.Port() != "" || len(strings.Split(answer, ".")) < 2 {
				return fmt.Errorf("please enter a valid hostname without protocol (https://), without path and without port, e.g. loft.my-domain.tld")
			}
			return nil
		},
	})
}

func ensureIngressController(ctx context.Context, kubeClient kubernetes.Interface, kubeContext string, log log.Logger) error {
	// first create an ingress controller
	const (
		YesOption = "Yes"
		NoOption  = "No, I already have an ingress controller installed."
	)

	answer, err := log.Question(&survey.QuestionOptions{
		Question:     "Ingress controller required. Should the nginx-ingress controller be installed?",
		DefaultValue: YesOption,
		Options: []string{
			YesOption,
			NoOption,
		},
	})
	if err != nil {
		return err
	}

	if answer == YesOption {
		args := []string{
			"install",
			"ingress-nginx",
			"ingress-nginx",
			"--repository-config=''",
			"--repo",
			"https://kubernetes.github.io/ingress-nginx",
			"--kube-context",
			kubeContext,
			"--namespace",
			"ingress-nginx",
			"--create-namespace",
			"--set-string",
			"controller.config.hsts=false",
			"--wait",
		}
		log.WriteString(logrus.InfoLevel, "\n")
		log.Infof("Executing command: helm %s\n", strings.Join(args, " "))
		log.Info("Waiting for ingress controller deployment, this can take several minutes...")
		helmCmd := exec.Command("helm", args...)
		output, err := helmCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("error during helm command: %s (%w)", string(output), err)
		}

		list, err := kubeClient.CoreV1().Secrets("ingress-nginx").List(ctx, metav1.ListOptions{
			LabelSelector: "name=ingress-nginx,owner=helm,status=deployed",
		})
		if err != nil {
			return err
		}

		if len(list.Items) == 1 {
			secret := list.Items[0]
			originalSecret := secret.DeepCopy()
			secret.Labels["loft.sh/app"] = "true"
			if secret.Annotations == nil {
				secret.Annotations = map[string]string{}
			}

			secret.Annotations["loft.sh/url"] = "https://kubernetes.github.io/ingress-nginx"
			originalJSON, err := json.Marshal(originalSecret)
			if err != nil {
				return err
			}
			modifiedJSON, err := json.Marshal(secret)
			if err != nil {
				return err
			}
			data, err := jsonpatch.CreateMergePatch(originalJSON, modifiedJSON)
			if err != nil {
				return err
			}
			_, err = kubeClient.CoreV1().Secrets(secret.Namespace).Patch(ctx, secret.Name, types.MergePatchType, data, metav1.PatchOptions{})
			if err != nil {
				return err
			}
		}

		log.Done("installed ingress-nginx to your kubernetes cluster!")
	}

	return nil
}

func deleteUser(ctx context.Context, restConfig *rest.Config, name string) error {
	loftClient, err := loftclientset.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	user, err := loftClient.StorageV1().Users().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil
	} else if len(user.Finalizers) > 0 {
		user.Finalizers = nil
		_, err = loftClient.StorageV1().Users().Update(ctx, user, metav1.UpdateOptions{})
		if err != nil {
			if kerrors.IsConflict(err) {
				return deleteUser(ctx, restConfig, name)
			}

			return err
		}
	}

	err = loftClient.StorageV1().Users().Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		return err
	}

	return nil
}

func ensureAdminPassword(ctx context.Context, kubeClient kubernetes.Interface, restConfig *rest.Config, password string, log log.Logger) (bool, error) {
	loftClient, err := loftclientset.NewForConfig(restConfig)
	if err != nil {
		return false, err
	}

	admin, err := loftClient.StorageV1().Users().Get(ctx, "admin", metav1.GetOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		return false, err
	} else if admin == nil {
		admin, err = loftClient.StorageV1().Users().Create(ctx, &storagev1.User{
			ObjectMeta: metav1.ObjectMeta{
				Name: "admin",
			},
			Spec: storagev1.UserSpec{
				Username: "admin",
				Email:    "test@domain.tld",
				Subject:  "admin",
				Groups:   []string{"system:masters"},
				PasswordRef: &storagev1.SecretRef{
					SecretName:      "loft-user-secret-admin",
					SecretNamespace: "loft",
					Key:             "password",
				},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			return false, err
		}
	} else if admin.Spec.PasswordRef == nil || admin.Spec.PasswordRef.SecretName == "" || admin.Spec.PasswordRef.SecretNamespace == "" {
		return false, nil
	}

	key := admin.Spec.PasswordRef.Key
	if key == "" {
		key = "password"
	}

	passwordHash := fmt.Sprintf("%x", sha256.Sum256([]byte(password)))

	secret, err := kubeClient.CoreV1().Secrets(admin.Spec.PasswordRef.SecretNamespace).Get(ctx, admin.Spec.PasswordRef.SecretName, metav1.GetOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		return false, err
	} else if err == nil {
		existingPasswordHash, keyExists := secret.Data[key]
		if keyExists {
			return (string(existingPasswordHash) != passwordHash), nil
		}

		secret.Data[key] = []byte(passwordHash)
		_, err = kubeClient.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
		if err != nil {
			return false, fmt.Errorf("update admin password secret: %w", err)
		}
		return false, nil
	}

	// create the password secret if it was not found, this can happen if you delete the loft namespace without deleting the admin user
	secret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      admin.Spec.PasswordRef.SecretName,
			Namespace: admin.Spec.PasswordRef.SecretNamespace,
		},
		Data: map[string][]byte{
			key: []byte(passwordHash),
		},
	}
	_, err = kubeClient.CoreV1().Secrets(secret.Namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return false, fmt.Errorf("create admin password secret: %w", err)
	}

	log.Info("recreated admin password secret")
	return false, nil
}

func getIngressHost(ctx context.Context, kubeClient kubernetes.Interface, namespace string) (string, error) {
	ingress, err := kubeClient.NetworkingV1().Ingresses(namespace).Get(ctx, "loft-ingress", metav1.GetOptions{})
	if err != nil {
		ingress, err := kubeClient.NetworkingV1beta1().Ingresses(namespace).Get(ctx, "loft-ingress", metav1.GetOptions{})
		if err != nil {
			return "", err
		} else {
			// find host
			for _, rule := range ingress.Spec.Rules {
				return rule.Host, nil
			}
		}
	} else {
		// find host
		for _, rule := range ingress.Spec.Rules {
			return rule.Host, nil
		}
	}

	return "", fmt.Errorf("couldn't find any host in loft ingress '%s/loft-ingress', please make sure you have not changed any deployed resources", namespace)
}

type version struct {
	Version string `json:"version"`
}

func isHostReachable(ctx context.Context, host string) (bool, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// we disable http2 as Kubernetes has problems with this
	transport.ForceAttemptHTTP2 = false
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	// wait until loft is reachable at the given url
	client := &http.Client{Transport: transport}
	url := "https://" + host + "/version"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("error creating request with context: %w", err)
	}
	resp, err := client.Do(req)
	if err == nil && resp.StatusCode == http.StatusOK {
		out, err := io.ReadAll(resp.Body)
		if err != nil {
			return false, nil
		}

		v := &version{}
		err = json.Unmarshal(out, v)
		if err != nil {
			return false, fmt.Errorf("error decoding response from %s: %w. Try running '%s --reset'", url, err, "devpod pro start")
		} else if v.Version == "" {
			return false, fmt.Errorf("unexpected response from %s: %s. Try running '%s --reset'", url, string(out), "devpod pro start")
		}

		return true, nil
	}

	return false, nil
}

func upgradeRelease(chartName, chartRepo, kubeContext, namespace string, extraArgs []string, log log.Logger) error {
	// now we install loft
	args := []string{
		"upgrade",
		defaultReleaseName,
		chartName,
		"--install",
		"--create-namespace",
		"--repository-config=''",
		"--kube-context",
		kubeContext,
		"--namespace",
		namespace,
	}
	if chartRepo != "" {
		args = append(args, "--repo", chartRepo)
	}
	args = append(args, extraArgs...)

	log.WriteString(logrus.InfoLevel, "\n")
	log.Infof("Executing command: helm %s\n", strings.Join(args, " "))
	log.Info("Waiting for helm command, this can take up to several minutes...")
	helmCmd := exec.Command("helm", args...)
	if chartRepo != "" {
		helmWorkDir, err := getHelmWorkdir(chartName)
		if err != nil {
			return err
		}

		helmCmd.Dir = helmWorkDir
	}
	output, err := helmCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error during helm command: %s (%w)", string(output), err)
	}

	log.Donef("DevPod Pro has been deployed to your cluster!")
	return nil
}

func getReleaseManifests(chartName, chartRepo, kubeContext, namespace string, extraArgs []string, _ log.Logger) (string, error) {
	args := []string{
		"template",
		defaultReleaseName,
		chartName,
		"--repository-config=''",
		"--kube-context",
		kubeContext,
		"--namespace",
		namespace,
	}
	if chartRepo != "" {
		args = append(args, "--repo", chartRepo)
	}
	args = append(args, extraArgs...)

	helmCmd := exec.Command("helm", args...)
	if chartRepo != "" {
		helmWorkDir, err := getHelmWorkdir(chartName)
		if err != nil {
			return "", err
		}

		helmCmd.Dir = helmWorkDir
	}
	output, err := helmCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("error during helm command: %s (%w)", string(output), err)
	}
	return string(output), nil
}

func getHelmWorkdir(chartName string) (string, error) {
	// If chartName folder exists, check temp dir next
	if _, err := os.Stat(chartName); err == nil {
		tempDir := os.TempDir()

		// If tempDir/chartName folder exists, create temp folder
		if _, err := os.Stat(path.Join(tempDir, chartName)); err == nil {
			tempDir, err = os.MkdirTemp(tempDir, chartName)
			if err != nil {
				return "", errors.New("problematic directory `" + chartName + "` found: please execute command in a different folder")
			}
		}

		// Use tempDir
		return tempDir, nil
	}

	// Use current workdir
	return "", nil
}

var (
	ErrMissingContainer = errors.New("missing container")
	ErrLoftNotReachable = errors.New("DevPod Pro is not reachable")
)

type ContainerDetails struct {
	NetworkSettings ContainerNetworkSettings `json:"NetworkSettings"`
	State           ContainerDetailsState    `json:"State"`
	ID              string                   `json:"ID,omitempty"`
	Created         string                   `json:"Created,omitempty"`
	Config          ContainerDetailsConfig   `json:"Config"`
}

type ContainerNetworkSettings struct {
	Ports map[string][]ContainerPort `json:"ports,omitempty"`
}

type ContainerPort struct {
	HostIP   string `json:"HostIp,omitempty"`
	HostPort string `json:"HostPort,omitempty"`
}

type ContainerDetailsConfig struct {
	Labels map[string]string `json:"Labels,omitempty"`
	Image  string            `json:"Image,omitempty"`
	User   string            `json:"User,omitempty"`
	Env    []string          `json:"Env,omitempty"`
}

type ContainerDetailsState struct {
	Status    string `json:"Status,omitempty"`
	StartedAt string `json:"StartedAt,omitempty"`
}

func WrapCommandError(stdout []byte, err error) error {
	if err == nil {
		return nil
	}

	return &Error{
		stdout: stdout,
		err:    err,
	}
}

type Error struct {
	err    error
	stdout []byte
}

func (e *Error) Error() string {
	message := ""
	if len(e.stdout) > 0 {
		message += string(e.stdout) + "\n"
	}

	var exitError *exec.ExitError
	if errors.As(e.err, &exitError) && len(exitError.Stderr) > 0 {
		message += string(exitError.Stderr) + "\n"
	}

	return message + e.err.Error()
}

func getMachineUID(log log.Logger) string {
	id, err := machineid.ID()
	if err != nil {
		id = "error"
		if log != nil {
			log.Debugf("Error retrieving machine uid: %v", err)
		}
	}
	// get $HOME to distinguish two users on the same machine
	// will be hashed later together with the ID
	home, err := util.UserHomeDir()
	if err != nil {
		home = "error"
		if log != nil {
			log.Debugf("Error retrieving machine home: %v", err)
		}
	}
	mac := hmac.New(sha256.New, []byte(id))
	mac.Write([]byte(home))
	return fmt.Sprintf("%x", mac.Sum(nil))
}
