package pro

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mgutz/ansi"
	"github.com/sirupsen/logrus"
	proflags "github.com/skevetter/devpod/cmd/pro/flags"
	"github.com/skevetter/devpod/pkg/platform"
	"github.com/skevetter/devpod/pkg/platform/client"
	"github.com/skevetter/log"
	"github.com/skevetter/log/survey"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/kubectl/pkg/util/term"
)

const LoftRouterDomainSecret = "loft-router-domain"
const passwordChangedHint = "(has been changed)"
const defaultUser = "admin"

const defaultReleaseName = "devpod-pro"

var defaultDeploymentName = "loft" // Need to update helm chart if we change this!

// StartCmd holds the login cmd flags
type StartCmd struct {
	proflags.GlobalFlags

	KubeClient       kubernetes.Interface
	Log              log.Logger
	RestConfig       *rest.Config
	Context          string
	Values           string
	LocalPort        string
	Version          string
	DockerImage      string
	Namespace        string
	Password         string
	Host             string
	Email            string
	ChartRepo        string
	Product          string
	ChartName        string
	ChartPath        string
	DockerArgs       []string
	Reset            bool
	NoPortForwarding bool
	NoTunnel         bool
	NoLogin          bool
	NoWait           bool
	Upgrade          bool
	ReuseValues      bool
	Docker           bool
}

// NewStartCmd creates a new command
func NewStartCmd(flags *proflags.GlobalFlags) *cobra.Command {
	cmd := &StartCmd{GlobalFlags: *flags,
		Product:   "devpod-pro",
		ChartName: "devpod-pro",
		Log:       log.Default,
	}
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start a Devpod Pro instance",
		RunE: func(_ *cobra.Command, args []string) error {
			return cmd.Run(context.Background())
		},
	}

	startCmd.Flags().BoolVar(&cmd.Docker, "docker", false, "If enabled will try to deploy DevPod Pro to the local docker installation.")
	startCmd.Flags().StringVar(&cmd.DockerImage, "docker-image", "", "The docker image to install.")
	startCmd.Flags().StringArrayVar(&cmd.DockerArgs, "docker-arg", []string{}, "Extra docker args")
	startCmd.Flags().StringVar(&cmd.Context, "context", "", "The kube context to use for installation")
	startCmd.Flags().StringVar(&cmd.Namespace, "namespace", "devpod-pro", "The namespace to install into")
	startCmd.Flags().StringVar(&cmd.Host, "host", "", "Provide a hostname to enable ingress and configure its hostname")
	startCmd.Flags().StringVar(&cmd.Password, "password", "", "The password to use for the admin account. (If empty this will be the namespace UID)")
	startCmd.Flags().StringVar(&cmd.Version, "version", "", "The version to install")
	startCmd.Flags().StringVar(&cmd.Values, "values", "", "Path to a file for extra helm chart values")
	startCmd.Flags().BoolVar(&cmd.ReuseValues, "reuse-values", true, "Reuse previous helm values on upgrade")
	startCmd.Flags().BoolVar(&cmd.Upgrade, "upgrade", false, "If true, will try to upgrade the release")
	startCmd.Flags().StringVar(&cmd.Email, "email", "", "The email to use for the installation")
	startCmd.Flags().BoolVar(&cmd.Reset, "reset", false, "If true, an existing instance will be deleted before installing DevPod Pro")
	startCmd.Flags().BoolVar(&cmd.NoWait, "no-wait", false, "If true, will not wait after installing it")
	startCmd.Flags().BoolVar(&cmd.NoTunnel, "no-tunnel", false, "If true, will not create a loft.host tunnel for this installation")
	startCmd.Flags().BoolVar(&cmd.NoLogin, "no-login", false, "If true, will not login to a DevPod Pro instance on start")
	startCmd.Flags().StringVar(&cmd.ChartPath, "chart-path", "", "The local chart path to deploy DevPod Pro")
	startCmd.Flags().StringVar(&cmd.ChartRepo, "chart-repo", "https://charts.loft.sh/", "The chart repo to deploy DevPod Pro")

	return startCmd
}

// Run runs the command logic
func (cmd *StartCmd) Run(ctx context.Context) error {
	if cmd.Docker {
		return cmd.startDocker(ctx)
	}

	// only set local port by default in kubernetes installation
	if cmd.LocalPort == "" {
		cmd.LocalPort = "9898"
	}

	err := cmd.prepare()
	if err != nil {
		return err
	}
	cmd.Log.WriteString(logrus.InfoLevel, "\n")

	// Uninstall already existing instance
	if cmd.Reset {
		err = uninstall(ctx, cmd.KubeClient, cmd.RestConfig, cmd.Context, cmd.Namespace, cmd.Log)
		if err != nil {
			return err
		}
	}

	// Is already installed?
	isInstalled, err := isAlreadyInstalled(ctx, cmd.KubeClient, cmd.Namespace)
	if err != nil {
		return err
	}

	// Use default password if none is set
	if cmd.Password == "" {
		defaultPassword, err := getDefaultPassword(ctx, cmd.KubeClient, cmd.Namespace)
		if err != nil {
			return err
		}

		cmd.Password = defaultPassword
	}

	// Upgrade Loft if already installed
	if isInstalled {
		return cmd.handleAlreadyExistingInstallation(ctx)
	}

	// Install Loft
	cmd.Log.Info("Welcome to DevPod Pro!")
	cmd.Log.Info("This installer will help you to get started.")

	// make sure we are ready for installing
	err = cmd.prepareInstall(ctx)
	if err != nil {
		return err
	}

	err = cmd.upgrade()
	if err != nil {
		return err
	}

	return cmd.success(ctx)
}

func (cmd *StartCmd) upgrade() error {
	extraArgs := []string{}
	if cmd.Host != "" || cmd.NoTunnel {
		extraArgs = append(extraArgs, "--set-string", "env.DISABLE_LOFT_ROUTER=true")
	}
	if cmd.Password != "" {
		extraArgs = append(extraArgs, "--set", "admin.password="+cmd.Password)
	}
	if cmd.Host != "" {
		extraArgs = append(extraArgs, "--set", "ingress.enabled=true", "--set", "ingress.host="+cmd.Host)
		extraArgs = append(extraArgs, "--set", "env.LOFT_HOST="+cmd.Host)
		extraArgs = append(extraArgs, "--set", "devpodIngress.enabled=true", "--set", "devpodIngress.host=*."+cmd.Host)
		extraArgs = append(extraArgs, "--set", "env.DEVPOD_SUBDOMAIN=*."+cmd.Host)
	}
	if cmd.Version != "" {
		extraArgs = append(extraArgs, "--version", cmd.Version)
	}
	if cmd.Product != "" {
		extraArgs = append(extraArgs, "--set", "product="+cmd.Product)
	}

	// Do not use --reuse-values if --reset flag is provided because this should be a new install and it will cause issues with `helm template`
	if !cmd.Reset && cmd.ReuseValues {
		extraArgs = append(extraArgs, "--reuse-values")
	}

	if cmd.Values != "" {
		absValuesPath, err := filepath.Abs(cmd.Values)
		if err != nil {
			return err
		}
		extraArgs = append(extraArgs, "--values", absValuesPath)
	}

	chartName := cmd.ChartPath
	chartRepo := ""
	if chartName == "" {
		chartName = cmd.ChartName
		chartRepo = cmd.ChartRepo
	}

	err := upgradeRelease(chartName, chartRepo, cmd.Context, cmd.Namespace, extraArgs, cmd.Log)
	if err != nil {
		if !cmd.Reset {
			return errors.New(err.Error() + fmt.Sprintf("\n\nIf want to purge and reinstall DevPod Pro, run: %s\n", ansi.Color("devpod pro start --reset", "green+b")))
		}

		// Try to purge Loft and retry install
		cmd.Log.Info("Trying to delete objects blocking current installation")

		manifests, err := getReleaseManifests(chartName, chartRepo, cmd.Context, cmd.Namespace, extraArgs, cmd.Log)
		if err != nil {
			return err
		}

		kubectlDelete := exec.Command("kubectl", "delete", "-f", "-", "--ignore-not-found=true", "--grace-period=0", "--force")

		buffer := bytes.Buffer{}
		buffer.Write([]byte(manifests))

		kubectlDelete.Stdin = &buffer
		kubectlDelete.Stdout = os.Stdout
		kubectlDelete.Stderr = os.Stderr

		// Ignoring potential errors here
		_ = kubectlDelete.Run()

		// Retry Loft installation
		err = upgradeRelease(chartName, chartRepo, cmd.Context, cmd.Namespace, extraArgs, cmd.Log)
		if err != nil {
			return errors.New(err.Error() + fmt.Sprintf("\n\nExisting installation failed. Reach out to get help:\n- via Slack: %s (fastest option)\n- via Online Chat: %s\n- via Email: %s\n", ansi.Color("https://slack.loft.sh/", "green+b"), ansi.Color("https://loft.sh/", "green+b"), ansi.Color("support@loft.sh", "green+b")))
		}
	}

	return nil
}

func (cmd *StartCmd) success(ctx context.Context) error {
	if cmd.NoWait {
		return nil
	}

	// wait until deployment is ready
	loftPod, err := cmd.waitForDeployment(ctx)
	if err != nil {
		return err
	}

	// check if installed locally
	isLocal := isInstalledLocally(ctx, cmd.KubeClient, cmd.Namespace)
	if isLocal {
		// check if loft domain secret is there
		if !cmd.NoTunnel {
			loftRouterDomain, err := cmd.pingLoftRouter(ctx, loftPod)
			if err != nil {
				cmd.Log.Errorf("Error retrieving loft router domain: %v", err)
				cmd.Log.Info("Fallback to use port-forwarding")
			} else if loftRouterDomain != "" {
				return cmd.successLoftRouter(loftRouterDomain)
			}
		}

		return cmd.successLocal()
	}

	// get login link
	cmd.Log.Info("Checking Loft status...")
	host, err := getIngressHost(ctx, cmd.KubeClient, cmd.Namespace)
	if err != nil {
		return err
	}

	// check if loft is reachable
	reachable, err := isHostReachable(ctx, host)
	if !reachable || err != nil {
		const (
			YesOption = "Yes"
			NoOption  = "No, please re-run the DNS check"
		)

		answer, err := cmd.Log.Question(&survey.QuestionOptions{
			Question:     "Unable to reach Loft at https://" + host + ". Do you want to start port-forwarding instead?",
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
			return cmd.successLocal()
		}
	}

	return cmd.successRemote(ctx, host)
}

func (cmd *StartCmd) successRemote(ctx context.Context, host string) error {
	printSuccess := func() {
		url := "https://" + host

		password := cmd.Password
		if password == "" {
			password = passwordChangedHint
		}

		cmd.Log.WriteString(logrus.InfoLevel, fmt.Sprintf(`


##########################   LOGIN   ############################

Username: `+ansi.Color("admin", "green+b")+`
Password: `+ansi.Color(password, "green+b")+`  # Change via UI or via: `+ansi.Color("devpod pro reset password", "green+b")+`

Login via UI:  %s
Login via CLI: %s

!!! You must accept the untrusted certificate in your browser !!!

Follow this guide to add a valid certificate: %s

#################################################################

DevPod Pro was successfully installed and can now be reached at: %s

Thanks for using DevPod Pro!
`,
			ansi.Color(url, "green+b"),
			ansi.Color("devpod pro login "+url, "green+b"),
			"https://loft.sh/docs/administration/ssl",
			url))
	}
	ready, err := isHostReachable(ctx, host)
	if err != nil {
		return err
	} else if ready {
		printSuccess()
		return nil
	}

	// Print DNS Configuration
	cmd.Log.WriteString(logrus.InfoLevel, `

###################################     DNS CONFIGURATION REQUIRED     ##################################

Create a DNS A-record for `+host+` with the EXTERNAL-IP of your nginx-ingress controller.
To find this EXTERNAL-IP, run the following command and look at the output:

> kubectl get services -n ingress-nginx
                                                     |---------------|
NAME                       TYPE           CLUSTER-IP | EXTERNAL-IP   |  PORT(S)                      AGE
ingress-nginx-controller   LoadBalancer   10.0.0.244 | XX.XXX.XXX.XX |  80:30984/TCP,443:31758/TCP   19m
                                                     |^^^^^^^^^^^^^^^|

EXTERNAL-IP may be 'pending' for a while until your cloud provider has created a new load balancer.

#########################################################################################################

The command will wait until DevPod Pro is reachable under the host.

`)

	cmd.Log.Info("Waiting for you to configure DNS, so DevPod Pro can be reached on https://" + host)
	err = wait.PollUntilContextTimeout(ctx, 5*time.Second, platform.Timeout(), true, func(ctx context.Context) (done bool, err error) {
		return isHostReachable(ctx, host)
	})
	if err != nil {
		return err
	}

	cmd.Log.Done("DevPod Pro is reachable at https://" + host)

	printSuccess()
	return nil
}
func (cmd *StartCmd) successLocal() error {
	url := "https://localhost:" + cmd.LocalPort

	if !cmd.NoLogin {
		err := cmd.login(url)
		if err != nil {
			return err
		}
	}

	password := cmd.Password
	if password == "" {
		password = passwordChangedHint
	}

	cmd.Log.WriteString(logrus.InfoLevel, fmt.Sprintf(`

##########################   LOGIN   ############################

Username: `+ansi.Color("admin", "green+b")+`
Password: `+ansi.Color(password, "green+b")+`  # Change via UI or via: `+ansi.Color("devpod pro reset password", "green+b")+`

Login via UI:  %s
Login via CLI: %s

!!! You must accept the untrusted certificate in your browser !!!

#################################################################

DevPod Pro was successfully installed.

Thanks for using DevPod Pro!
`, ansi.Color(url, "green+b"), ansi.Color("devpod pro login"+" --insecure "+url, "green+b")))
	blockChan := make(chan bool)
	<-blockChan
	return nil
}

func (cmd *StartCmd) prepareInstall(ctx context.Context) error {
	// delete admin user & secret
	return uninstall(ctx, cmd.KubeClient, cmd.RestConfig, cmd.Context, cmd.Namespace, log.Discard)
}

func (cmd *StartCmd) prepare() error {
	loader, err := client.NewClientFromPath(cmd.Config)
	if err != nil {
		return err
	}
	loftConfig := loader.Config()

	// first load the kube config
	kubeClientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{})

	// load the raw config
	kubeConfig, err := kubeClientConfig.RawConfig()
	if err != nil {
		return fmt.Errorf("there is an error loading your current kube config (%w), please make sure you have access to a kubernetes cluster and the command `kubectl get namespaces` is working", err)
	}

	// we switch the context to the install config
	contextToLoad := kubeConfig.CurrentContext
	if cmd.Context != "" {
		contextToLoad = cmd.Context
	} else if loftConfig.LastInstallContext != "" && loftConfig.LastInstallContext != contextToLoad {
		contextToLoad, err = cmd.Log.Question(&survey.QuestionOptions{
			Question:     "Seems like you try to use 'devpod pro start' with a different kubernetes context than before. Please choose which kubernetes context you want to use",
			DefaultValue: contextToLoad,
			Options:      []string{contextToLoad, loftConfig.LastInstallContext},
		})
		if err != nil {
			return err
		}
	}
	cmd.Context = contextToLoad

	loftConfig.LastInstallContext = contextToLoad
	_ = loader.Save()

	// kube client config
	kubeClientConfig = clientcmd.NewNonInteractiveClientConfig(kubeConfig, contextToLoad, &clientcmd.ConfigOverrides{}, clientcmd.NewDefaultClientConfigLoadingRules())

	// test for helm and kubectl
	_, err = exec.LookPath("helm")
	if err != nil {
		return fmt.Errorf("seems like helm is not installed. Helm is required for the installation of loft. Please visit https://helm.sh/docs/intro/install/ for install instructions")
	}

	output, err := exec.Command("helm", "version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("seems like there are issues with your helm client: \n\n%s", output)
	}

	_, err = exec.LookPath("kubectl")
	if err != nil {
		return fmt.Errorf("seems like kubectl is not installed. Kubectl is required for the installation of loft. Please visit https://kubernetes.io/docs/tasks/tools/install-kubectl/ for install instructions")
	}

	output, err = exec.Command("kubectl", "version", "--context", contextToLoad).CombinedOutput()
	if err != nil {
		return fmt.Errorf("seems like kubectl cannot connect to your Kubernetes cluster: \n\n%s", output)
	}

	cmd.RestConfig, err = kubeClientConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("there is an error loading your current kube config (%w), please make sure you have access to a kubernetes cluster and the command `kubectl get namespaces` is working", err)
	}
	cmd.KubeClient, err = kubernetes.NewForConfig(cmd.RestConfig)
	if err != nil {
		return fmt.Errorf("there is an error loading your current kube config (%w), please make sure you have access to a kubernetes cluster and the command `kubectl get namespaces` is working", err)
	}

	// Check if cluster has RBAC correctly configured
	_, err = cmd.KubeClient.RbacV1().ClusterRoles().Get(context.Background(), "cluster-admin", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error retrieving cluster role 'cluster-admin': %w. Please make sure RBAC is correctly configured in your cluster", err)
	}

	return nil
}

func (cmd *StartCmd) handleAlreadyExistingInstallation(ctx context.Context) error {
	enableIngress := false

	// Only ask if ingress should be enabled if --upgrade flag is not provided
	if !cmd.Upgrade && term.IsTerminal(os.Stdin) {
		cmd.Log.Info("Existing instance found.")

		// Check if Loft is installed in a local cluster
		isLocal := isInstalledLocally(ctx, cmd.KubeClient, cmd.Namespace)

		// Skip question if --host flag is provided
		if cmd.Host != "" {
			enableIngress = true
		}

		if enableIngress {
			if isLocal {
				// Confirm with user if this is a local cluster
				const (
					YesOption = "Yes"
					NoOption  = "No, my cluster is running not locally (GKE, EKS, Bare Metal, etc.)"
				)

				answer, err := cmd.Log.Question(&survey.QuestionOptions{
					Question:     "Seems like your cluster is running locally (docker desktop, minikube, kind etc.). Is that correct?",
					DefaultValue: YesOption,
					Options: []string{
						YesOption,
						NoOption,
					},
				})
				if err != nil {
					return err
				}

				isLocal = answer == YesOption
			}

			if isLocal {
				// Confirm with user if ingress should be installed in local cluster
				var (
					YesOption = "Yes, enable the ingress anyway"
					NoOption  = "No"
				)

				answer, err := cmd.Log.Question(&survey.QuestionOptions{
					Question:     "Enabling ingress is usually only useful for remote clusters. Do you still want to deploy the ingress to your local cluster?",
					DefaultValue: NoOption,
					Options: []string{
						NoOption,
						YesOption,
					},
				})
				if err != nil {
					return err
				}

				enableIngress = answer == YesOption
			}
		}

		// Check if we need to enable ingress
		if enableIngress {
			// Ask for hostname if --host flag is not provided
			if cmd.Host == "" {
				host, err := enterHostNameQuestion(cmd.Log)
				if err != nil {
					return err
				}

				cmd.Host = host
			} else {
				cmd.Log.Info("Will enable an ingress with hostname: " + cmd.Host)
			}

			if term.IsTerminal(os.Stdin) {
				err := ensureIngressController(ctx, cmd.KubeClient, cmd.Context, cmd.Log)
				if err != nil {
					return fmt.Errorf("install ingress controller: %w", err)
				}
			}
		}
	}

	// Only upgrade if --upgrade flag is present or user decided to enable ingress
	if cmd.Upgrade || enableIngress {
		err := cmd.upgrade()
		if err != nil {
			return err
		}
	}

	return cmd.success(ctx)
}

func (cmd *StartCmd) waitForDeployment(ctx context.Context) (*corev1.Pod, error) {
	// wait for loft pod to start
	cmd.Log.Info("waiting for DevPod Pro pod to be running")
	loftPod, err := platform.WaitForPodReady(ctx, cmd.KubeClient, cmd.Namespace, cmd.Log)
	cmd.Log.Donef("release Pod started")
	if err != nil {
		return nil, err
	}

	// ensure user admin secret is there
	isNewPassword, err := ensureAdminPassword(ctx, cmd.KubeClient, cmd.RestConfig, cmd.Password, cmd.Log)
	if err != nil {
		return nil, err
	}

	// If password is different than expected
	if isNewPassword {
		cmd.Password = ""
	}

	return loftPod, nil
}

func (cmd *StartCmd) pingLoftRouter(ctx context.Context, loftPod *corev1.Pod) (string, error) {
	loftRouterSecret, err := cmd.KubeClient.CoreV1().Secrets(loftPod.Namespace).Get(ctx, LoftRouterDomainSecret, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			return "", nil
		}

		return "", fmt.Errorf("find loft router domain secret: %w", err)
	} else if loftRouterSecret.Data == nil || len(loftRouterSecret.Data["domain"]) == 0 {
		return "", nil
	}

	// get the domain from secret
	loftRouterDomain := string(loftRouterSecret.Data["domain"])

	// wait until loft is reachable at the given url
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
	cmd.Log.Infof("Waiting until DevPod Pro is reachable at https://%s", loftRouterDomain)
	err = wait.PollUntilContextTimeout(ctx, time.Second*3, time.Minute*5, true, func(ctx context.Context) (bool, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+loftRouterDomain+"/version", nil)
		if err != nil {
			return false, nil
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return false, nil
		}

		return resp.StatusCode == http.StatusOK, nil
	})
	if err != nil {
		return "", err
	}

	return loftRouterDomain, nil
}

func (cmd *StartCmd) successLoftRouter(url string) error {
	if !cmd.NoLogin {
		err := cmd.login(url)
		if err != nil {
			return err
		}
	}

	url = "https://" + url

	password := cmd.Password
	if password == "" {
		password = passwordChangedHint
	}

	cmd.Log.WriteString(logrus.InfoLevel, fmt.Sprintf(`


##########################   LOGIN   ############################

Username: `+ansi.Color("admin", "green+b")+`
Password: `+ansi.Color(password, "green+b")+`  # Change via UI or via: `+ansi.Color("devpod pro reset password", "green+b")+`

Login via UI:  %s
Login via CLI: %s

#################################################################

DevPod Pro was successfully installed and can now be reached at: %s

Thanks for using DevPod Pro!
`,
		ansi.Color(url, "green+b"),
		ansi.Color("devpod pro login"+" "+url, "green+b"),
		url,
	))
	return nil
}
