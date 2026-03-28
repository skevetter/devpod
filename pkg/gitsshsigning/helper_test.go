package gitsshsigning

import (
	"strings"
	"testing"
)

func TestRemoveSignatureHelper_PreservesUnrelatedGpgConfig(t *testing.T) {
	input := strings.Join([]string{
		"[user]", "\tname = Test User", "\temail = test@example.com",
		"[gpg \"ssh\"]", "\tprogram = devpod-ssh-signature",
		"[gpg]", "\tformat = ssh", "\tprogram = /usr/bin/gpg2",
		"[commit]", "\tgpgsign = true",
		"[user]", "\tsigningkey = /path/to/key",
	}, "\n")

	result := removeSignatureHelper(input)

	if strings.Contains(result, "devpod-ssh-signature") {
		t.Errorf("expected devpod-ssh-signature to be removed, got:\n%s", result)
	}
	if !strings.Contains(result, "[user]") {
		t.Errorf("expected [user] section to be preserved, got:\n%s", result)
	}
	if !strings.Contains(result, "[commit]") {
		t.Errorf("expected [commit] section to be preserved, got:\n%s", result)
	}
	if !strings.Contains(result, "program = /usr/bin/gpg2") {
		t.Errorf("expected unrelated gpg program to be preserved, got:\n%s", result)
	}
	if strings.Contains(result, "format = ssh") {
		t.Errorf("expected 'format = ssh' to be removed, got:\n%s", result)
	}
}

func TestRemoveSignatureHelper_RemovesDevpodSections(t *testing.T) {
	input := strings.Join([]string{
		"[user]", "\tname = Test User",
		"[gpg \"ssh\"]", "\tprogram = devpod-ssh-signature",
		"[gpg]", "\tformat = ssh",
		"[user]", "\tsigningkey = /path/to/key",
	}, "\n")

	result := removeSignatureHelper(input)

	if strings.Contains(result, "devpod-ssh-signature") {
		t.Errorf("expected devpod-ssh-signature to be removed, got:\n%s", result)
	}
	if strings.Contains(result, "format = ssh") {
		t.Errorf("expected 'format = ssh' under [gpg] to be removed, got:\n%s", result)
	}
	if !strings.Contains(result, "Test User") {
		t.Errorf("expected user name to be preserved, got:\n%s", result)
	}
}

func TestRemoveSignatureHelper_NoGpgSections(t *testing.T) {
	input := "[user]\n\tname = Test User\n\temail = test@example.com"

	result := removeSignatureHelper(input)

	if result != input {
		t.Errorf(
			"expected input to be unchanged when no gpg sections exist.\nExpected:\n%s\nGot:\n%s",
			input,
			result,
		)
	}
}
