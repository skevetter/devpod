package inject

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/loft-sh/log"
	"github.com/skevetter/devpod/pkg/command"
)

//go:embed inject.sh
var Script string

type ExecFunc func(ctx context.Context, command string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error

type LocalFile func(arm bool) (io.ReadCloser, error)

type injectResult struct {
	wasExecuted bool
	err         error
}

func InjectAndExecute(
	ctx context.Context,
	exec ExecFunc,
	localFile LocalFile,
	scriptParams *Params,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	timeout time.Duration,
	log log.Logger,
) (bool, error) {
	scriptRawCode, err := GenerateScript(Script, scriptParams)
	if err != nil {
		return true, err
	}

	log.Debugf("execute inject script")
	if scriptParams.PreferAgentDownload {
		log.Debugf("download agent from %s", scriptParams.DownloadURLs.Base)
	}

	defer log.Debugf("done injecting")

	// start script
	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		return true, err
	}
	defer func() { _ = stdinWriter.Close() }()

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		return true, err
	}

	// delayed stderr
	delayedStderr := newDelayedWriter(stderr)

	// check if context is done
	select {
	case <-ctx.Done():
		return true, context.Canceled
	default:
	}

	// create cancel context
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// start execution of inject.sh
	execErrChan := make(chan error, 1)
	go func() {
		defer func() { _ = stdoutWriter.Close() }()
		defer log.Debugf("done exec")

		err := exec(cancelCtx, scriptRawCode, stdinReader, stdoutWriter, delayedStderr)
		if err != nil && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "signal: ") {
			execErrChan <- command.WrapCommandError(delayedStderr.Buffer(), err)
		} else {
			execErrChan <- nil
		}
	}()

	// inject file
	injectChan := make(chan injectResult, 1)
	go func() {
		defer func() { _ = stdinWriter.Close() }()
		defer log.Debugf("done inject")

		wasExecuted, err := inject(localFile, stdinWriter, stdin, stdoutReader, stdout, delayedStderr, timeout, log)
		injectChan <- injectResult{
			wasExecuted: wasExecuted,
			err:         command.WrapCommandError(delayedStderr.Buffer(), err),
		}
	}()

	// wait here
	var result injectResult
	select {
	case err = <-execErrChan:
		result = <-injectChan
	case result = <-injectChan:
		// we don't wait for the command termination here and will just retry on error
	}

	// prefer result error
	if result.err != nil {
		return result.wasExecuted, result.err
	} else if err != nil {
		return result.wasExecuted, err
	} else if result.wasExecuted || scriptParams.Command == "" {
		return result.wasExecuted, nil
	}

	log.Debugf("Rerun command as binary was injected")
	delayedStderr.Start()
	return true, exec(ctx, scriptParams.Command, stdin, stdout, delayedStderr)
}

func inject(
	localFile LocalFile,
	stdin io.WriteCloser,
	stdinOut io.Reader,
	stdout io.ReadCloser,
	stdoutOut io.Writer,
	delayedStderr *delayedWriter,
	timeout time.Duration,
	log log.Logger,
) (bool, error) {
	// wait until we read start
	var line string
	errChan := make(chan error)
	go func() {
		var err error
		line, err = readLine(stdout)
		errChan <- err
	}()

	// wait for line to be read
	err := waitForMessage(errChan, timeout)
	if err != nil {
		return false, err
	}

	err = performMutualHandshake(line, stdin)
	if err != nil {
		return false, err
	}

	// wait until we read something
	line, err = readLine(stdout)
	if err != nil {
		return false, err
	}
	log.Debugf("Received line after pong: %v", line)

	lineStr := strings.TrimSpace(line)
	if isInjectingOfBinaryNeeded(lineStr) {
		log.Debugf("Inject binary")
		defer log.Debugf("Done injecting binary")

		fileReader, err := getFileReader(localFile, lineStr)
		if err != nil {
			return false, err
		}
		defer func() { _ = fileReader.Close() }()
		err = injectBinary(fileReader, stdin, stdout, log)
		if err != nil {
			return false, err
		}
		_ = stdout.Close()
		// start exec with command
		return false, nil
	} else if lineStr != "done" {
		return false, fmt.Errorf("unexpected message during inject: %s", lineStr)
	}

	if stdoutOut == nil {
		stdoutOut = io.Discard
	}
	if stdinOut == nil {
		stdinOut = bytes.NewReader(nil)
	}

	// now pipe reader into stdout
	delayedStderr.Start()
	return true, pipe(
		stdin, stdinOut,
		stdoutOut, stdout,
	)
}

func isInjectingOfBinaryNeeded(lineStr string) bool {
	return strings.HasPrefix(lineStr, "ARM-")
}

func getFileReader(localFile LocalFile, lineStr string) (io.ReadCloser, error) {
	isArm := strings.TrimPrefix(lineStr, "ARM-") == "true"
	return localFile(isArm)
}

func performMutualHandshake(line string, stdin io.WriteCloser) error {
	// check for string
	if strings.TrimSpace(line) != "ping" {
		return fmt.Errorf("unexpected start line: %v", line)
	}

	// send our response
	_, err := stdin.Write([]byte("pong\n"))
	if err != nil {
		return fmt.Errorf("write to stdin %w", err)
	}

	// successful handshake
	return nil
}

type ProgressReader struct {
	reader    io.Reader
	total     int64
	current   int64
	startTime time.Time
	log       log.Logger
	mu        sync.Mutex
}

func NewProgressReader(reader io.Reader, total int64, log log.Logger) *ProgressReader {
	return &ProgressReader{
		reader:    reader,
		total:     total,
		startTime: time.Now(),
		log:       log,
	}
}

func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.mu.Lock()
		pr.current += int64(n)
		current := pr.current
		pr.mu.Unlock()

		// Log progress every 5MB or at completion
		if current%5242880 == 0 || current == pr.total || err == io.EOF {
			elapsed := time.Since(pr.startTime)
			speed := float64(current) / elapsed.Seconds() / 1024 / 1024 // MB/s
			percent := float64(current) / float64(pr.total) * 100
			pr.log.Infof("copying agent binary %.1f MB / %.1f MB (%.0f%%) %.1f MB/s",
				float64(current)/1024/1024, float64(pr.total)/1024/1024, percent, speed)
		}
	}
	return n, err
}

func injectBinary(
	fileReader io.ReadCloser,
	stdin io.WriteCloser,
	stdout io.ReadCloser,
	log log.Logger,
) error {
	// Get file size for progress tracking
	var totalSize int64
	if seeker, ok := fileReader.(io.Seeker); ok {
		if size, err := seeker.Seek(0, io.SeekEnd); err == nil {
			totalSize = size
			seeker.Seek(0, io.SeekStart)
		}
	}

	// Wrap with progress tracker if we have size
	var reader io.Reader = fileReader
	if totalSize > 0 {
		log.Infof("copying agent binary to remote (%.1f MB)", float64(totalSize)/1024/1024)
		reader = NewProgressReader(fileReader, totalSize, log)
	}

	// copy into writer with retry logic
	const maxRetries = 3
	const baseDelay = 2 * time.Second

	var copyErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Reset file position for retry
		if seeker, ok := fileReader.(io.Seeker); ok {
			seeker.Seek(0, io.SeekStart)
		}

		if attempt > 1 {
			log.Infof("Retrying binary copy (attempt %d/%d)", attempt, maxRetries)
			// Recreate progress reader for retry
			if totalSize > 0 {
				reader = NewProgressReader(fileReader, totalSize, log)
			}
		}

		_, copyErr = io.Copy(stdin, reader)
		if copyErr == nil {
			break
		}

		if attempt < maxRetries {
			delay := time.Duration(attempt) * baseDelay
			log.Warnf("Binary copy failed: %v. Retrying in %v...", copyErr, delay)
			time.Sleep(delay)
		}
	}

	if copyErr != nil {
		return fmt.Errorf("binary copy failed after %d attempts: %w", maxRetries, copyErr)
	}

	// close stdin
	_ = stdin.Close()

	if totalSize > 0 {
		log.Infof("copy agent binary to remote completed")
	}

	// wait for done
	line, err := readLine(stdout)
	if err != nil {
		return err
	} else if strings.TrimSpace(line) != "done" {
		return fmt.Errorf("unexpected line during inject: %s", line)
	}
	return nil
}

func waitForMessage(errChannel chan error, timeout time.Duration) error {
	select {
	case err := <-errChannel:
		return err
	case <-time.After(timeout):
		return context.DeadlineExceeded
	}
}

func readLine(reader io.Reader) (string, error) {
	// we always only read a single byte
	buf := make([]byte, 1)
	str := ""
	for {
		n, err := reader.Read(buf)
		if err != nil {
			return "", err
		} else if n == 0 {
			continue
		} else if buf[0] == '\n' {
			return str, nil
		}

		str += string(buf)
	}
}

func pipe(toStdin io.Writer, fromStdin io.Reader, toStdout io.Writer, fromStdout io.Reader) error {
	errChan := make(chan error, 2)
	go func() {
		_, err := io.Copy(toStdout, fromStdout)
		errChan <- err
	}()
	go func() {
		_, err := io.Copy(toStdin, fromStdin)
		errChan <- err
	}()
	return <-errChan
}
