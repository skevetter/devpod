package credentials

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"

	"github.com/sirupsen/logrus"
	"github.com/skevetter/devpod/pkg/agent/tunnel"
	"github.com/skevetter/log"
)

const DefaultPort = "12049"
const CredentialsServerPortEnv = "DEVPOD_CREDENTIALS_SERVER_PORT"

func RunCredentialsServer(
	ctx context.Context,
	port int,
	client tunnel.TunnelClient,
	log log.Logger,
) error {
	var handler http.Handler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		log.WithFields(logrus.Fields{"path": request.URL.Path}).Debug("incoming client connection")
		switch request.URL.Path {
		case "/git-credentials":
			err := handleGitCredentialsRequest(ctx, writer, request, client, log)
			if err != nil {
				http.Error(writer, err.Error(), http.StatusInternalServerError)
				return
			}
		case "/docker-credentials":
			err := handleDockerCredentialsRequest(ctx, writer, request, client, log)
			if err != nil {
				http.Error(writer, err.Error(), http.StatusInternalServerError)
				return
			}
		case "/git-ssh-signature":
			err := handleGitSSHSignatureRequest(ctx, writer, request, client, log)
			if err != nil {
				http.Error(writer, err.Error(), http.StatusInternalServerError)
				return
			}
		case "/loft-platform-credentials":
			err := handleLoftPlatformCredentialsRequest(ctx, writer, request, client, log)
			if err != nil {
				http.Error(writer, err.Error(), http.StatusInternalServerError)
			}
		case "/gpg-public-keys":
			err := handleGPGPublicKeysRequest(ctx, writer, request, client, log)
			if err != nil {
				http.Error(writer, err.Error(), http.StatusInternalServerError)
			}
		}
	})

	addr := net.JoinHostPort("localhost", strconv.Itoa(port))
	srv := &http.Server{Addr: addr, Handler: handler}

	errChan := make(chan error, 1)
	go func() {
		log.WithFields(logrus.Fields{"port": port}).Debug("credentials server started")

		// always returns error. ErrServerClosed on graceful close
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			errChan <- err
		} else {
			errChan <- nil
		}
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		_ = srv.Close()
		return nil
	}
}

func GetPort() (int, error) {
	strPort := cmp.Or(os.Getenv(CredentialsServerPortEnv), DefaultPort)
	port, err := strconv.Atoi(strPort)
	if err != nil {
		return 0, fmt.Errorf("convert port %s: %w", strPort, err)
	}

	return port, nil
}

func handleDockerCredentialsRequest(ctx context.Context, writer http.ResponseWriter, request *http.Request, client tunnel.TunnelClient, log log.Logger) error {
	out, err := io.ReadAll(request.Body)
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}

	log.WithFields(logrus.Fields{"data": string(out)}).Debug("received docker credentials post data")
	response, err := client.DockerCredentials(ctx, &tunnel.Message{Message: string(out)})
	if err != nil {
		return fmt.Errorf("get docker credentials response: %w", err)
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte(response.Message))
	log.WithFields(logrus.Fields{"bytes": len(response.Message)}).Debug("wrote docker credentials response")
	return nil
}

func handleGitCredentialsRequest(ctx context.Context, writer http.ResponseWriter, request *http.Request, client tunnel.TunnelClient, log log.Logger) error {
	out, err := io.ReadAll(request.Body)
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}

	log.WithFields(logrus.Fields{"data": string(out)}).Debug("received git credentials post data")
	response, err := client.GitCredentials(ctx, &tunnel.Message{Message: string(out)})
	if err != nil {
		log.WithFields(logrus.Fields{"error": err}).Debug("error receiving git credentials")
		return fmt.Errorf("get git credentials response: %w", err)
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte(response.Message))
	log.WithFields(logrus.Fields{"bytes": len(response.Message)}).Debug("wrote git credentials response")
	return nil
}

func handleGitSSHSignatureRequest(ctx context.Context, writer http.ResponseWriter, request *http.Request, client tunnel.TunnelClient, log log.Logger) error {
	out, err := io.ReadAll(request.Body)
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}

	log.WithFields(logrus.Fields{"data": string(out)}).Debug("received git SSH signature post data")
	response, err := client.GitSSHSignature(ctx, &tunnel.Message{Message: string(out)})
	if err != nil {
		log.WithFields(logrus.Fields{"error": err}).Error("error receiving git SSH signature")
		return fmt.Errorf("get git ssh signature: %w", err)
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte(response.Message))
	log.WithFields(logrus.Fields{"bytes": len(response.Message)}).Debug("wrote git SSH signature response")
	return nil
}

func handleLoftPlatformCredentialsRequest(ctx context.Context, writer http.ResponseWriter, request *http.Request, client tunnel.TunnelClient, log log.Logger) error {
	out, err := io.ReadAll(request.Body)
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}

	log.WithFields(logrus.Fields{"data": string(out)}).Debug("received loft platform credentials post data")
	response, err := client.LoftConfig(ctx, &tunnel.Message{Message: string(out)})
	if err != nil {
		log.WithFields(logrus.Fields{"error": err}).Error("error receiving platform credentials")
		return fmt.Errorf("get platform credentials: %w", err)
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte(response.Message))
	log.WithFields(logrus.Fields{"bytes": len(response.Message)}).Debug("wrote platform credentials response")
	return nil
}

func handleGPGPublicKeysRequest(ctx context.Context, writer http.ResponseWriter, request *http.Request, client tunnel.TunnelClient, log log.Logger) error {
	response, err := client.GPGPublicKeys(ctx, &tunnel.Message{})
	if err != nil {
		log.WithFields(logrus.Fields{"error": err}).Error("error receiving GPG public keys")
		return fmt.Errorf("get gpg public keys: %w", err)
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte(response.Message))
	log.WithFields(logrus.Fields{"bytes": len(response.Message)}).Debug("wrote GPG public keys response")
	return nil
}
