package agent

import (
	"strings"
	"testing"
)

func TestNormalizeChatAttachments(t *testing.T) {
	attachments, err := normalizeChatAttachments([]ChatAttachment{{
		Name: "capture.png", MIMEType: "IMAGE/PNG", Data: "aGVsbG8=",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(attachments) != 1 || attachments[0].MIMEType != "image/png" || attachments[0].Name != "capture.png" {
		t.Fatalf("attachments = %#v", attachments)
	}

	_, err = normalizeChatAttachments([]ChatAttachment{{
		Name: "capture.bmp", MIMEType: "image/bmp", Data: "aGVsbG8=",
	}})
	if err == nil || !strings.Contains(err.Error(), "不支持图片格式") {
		t.Fatalf("unsupported image error = %v", err)
	}
}

func TestChatAttachmentsPersistAcrossSessionReload(t *testing.T) {
	sessions := t.TempDir()
	config := ServiceConfig{SkillsDir: t.TempDir(), SessionsDir: sessions}
	svc := NewService(config)
	attachment := ChatAttachment{Name: "capture.png", MIMEType: "image/png", Data: "aGVsbG8="}
	svc.recordUserEventWithAttachments("看看这个", []ChatAttachment{attachment})

	history := svc.GetSessionMessages()
	if len(history) != 1 || len(history[0].Attachments) != 1 || history[0].Attachments[0] != attachment {
		t.Fatalf("history attachments = %#v", history)
	}

	reloaded := NewService(config)
	if len(reloaded.sessionMessages) != 1 || len(reloaded.sessionMessages[0].Attachments) != 1 {
		t.Fatalf("restored session = %#v", reloaded.sessionMessages)
	}
	restored := reloaded.sessionMessages[0].Attachments[0]
	if restored.Name != attachment.Name || restored.MIMEType != attachment.MIMEType || restored.Data != attachment.Data {
		t.Fatalf("restored attachment = %#v", restored)
	}
}
