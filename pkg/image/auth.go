package image

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	ecr "github.com/awslabs/amazon-ecr-credential-helper/ecr-login"
	"github.com/chrismellard/docker-credential-acr-env/pkg/credhelper"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/go-containerregistry/pkg/authn"
	kubernetesauth "github.com/google/go-containerregistry/pkg/authn/kubernetes"
	"github.com/google/go-containerregistry/pkg/v1/google"
)

var (
	amazonKeychain authn.Keychain = authn.NewKeychainFromHelper(ecr.NewECRHelper(ecr.WithLogger(io.Discard)))
	azureKeychain  authn.Keychain = authn.NewKeychainFromHelper(credhelper.NewACRCredentialsHelper())
)

const tokenFileLocation = "/var/run/secrets/kubernetes.io/serviceaccount/token"

// See https://github.com/kubernetes/kubernetes/blob/30ae12d018697d3c5f04e225b11f242f5310e097/pkg/serviceaccount/claims.go#L55
type privateClaims struct {
	jwt.RegisteredClaims
	Kubernetes kubernetesClaim `json:"kubernetes.io"`
}

type kubernetesClaim struct {
	Namespace string           `json:"namespace,omitempty"`
	Svcacct   ref              `json:"serviceaccount"`
	Pod       *ref             `json:"pod,omitempty"`
	Secret    *ref             `json:"secret,omitempty"`
	Node      *ref             `json:"node,omitempty"`
	WarnAfter *jwt.NumericDate `json:"warnafter,omitempty"`
}

type ref struct {
	Name string `json:"name,omitempty"`
	UID  string `json:"uid,omitempty"`
}

func GetKeychain(ctx context.Context) (authn.Keychain, error) {
	tokenBytes, err := os.ReadFile(tokenFileLocation)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// we're not in a kubernetes pod, use default keychain
			return authn.DefaultKeychain, nil
		}

		return nil, fmt.Errorf("failed to read kubernetes service account token: %w", err)
	}

	// in-cluster auth
	m, err := getPodMetadata(tokenBytes)
	if err != nil {
		return nil, err
	}
	k8sKeychain, err := kubernetesauth.NewInCluster(ctx, kubernetesauth.Options{
		ServiceAccountName: m.serviceAccountName,
		Namespace:          m.namespace,
	})
	if err != nil {
		return nil, err
	}

	// add default keychains
	keyChains := []authn.Keychain{
		k8sKeychain,
		google.Keychain,
		amazonKeychain,
	}

	// check if we should add azure keychain
	if os.Getenv("AZURE_CLIENT_ID") != "" && os.Getenv("AZURE_TENANT_ID") != "" {
		keyChains = append(keyChains, azureKeychain)
	}
	keyChains = append(keyChains, authn.DefaultKeychain)

	// Order matters here: We want to go through all of the cloud provider keychains before we hit the default keychain (docker config.json)
	return authn.NewMultiKeychain(
		keyChains...,
	), nil
}

type podMetadata struct {
	serviceAccountName string
	namespace          string
}

func getPodMetadata(token []byte) (podMetadata, error) {
	tokenStr := strings.TrimSpace(string(token))
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	claims := &privateClaims{}

	_, err := parser.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (any, error) {
		return nil, nil
	})
	if err != nil {
		return podMetadata{}, fmt.Errorf("failed to parse kubernetes service account token: %w", err)
	}

	kubeClaim := claims.Kubernetes
	if kubeClaim.Namespace == "" || kubeClaim.Svcacct.Name == "" {
		return podMetadata{}, fmt.Errorf("failed to retrieve pod metadata from kubernetes service account token")
	}

	return podMetadata{
		namespace:          kubeClaim.Namespace,
		serviceAccountName: kubeClaim.Svcacct.Name,
	}, nil
}
