package agent_test

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	devopsv1 "silent-devops/api/devops/v1"
	"silent-devops/internal/agent"
	"silent-devops/internal/maintenance"
	sshmanager "silent-devops/internal/ssh"
)

func TestHandlerExecutesTypedJobAndReturnsResult(t *testing.T) {
	sent := make(chan *devopsv1.AgentMessage, 1)
	h := agent.Handler{Dispatcher: maintenance.Dispatcher{Runner: maintenance.Runner{MaxOutputBytes: 1024}, Timeout: time.Second}, Send: func(m *devopsv1.AgentMessage) error { sent <- m; return nil }}
	job := &devopsv1.Job{Id: "j", AgentId: "a", DeadlineUnixMs: time.Now().Add(time.Minute).UnixMilli(), Attempt: 1, DispatchId: "d", Operation: &devopsv1.Job_SafeOperation{SafeOperation: &devopsv1.SafeOperation{Operation: &devopsv1.TypedOperation{Request: &devopsv1.TypedOperation_ProcessList{ProcessList: &devopsv1.ProcessListRequest{Limit: 1}}}}}}
	if err := h.Handle(context.Background(), &devopsv1.ValidatorMessage{Payload: &devopsv1.ValidatorMessage_Job{Job: job}}); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-sent:
		if message.GetJobResult().JobId != "j" {
			t.Fatal("wrong result")
		}
	case <-time.After(time.Second):
		t.Fatal("result not sent")
	}
}
func TestSSHHandlerSendsAgentHostKeyOnReady(t *testing.T) {
	dir := t.TempDir()
	sent := make(chan *devopsv1.AgentMessage, 1)
	h := &agent.SSHHandler{
		KeyStore: sshmanager.KeyStore{Path: dir + "/authorized_keys"},
		HostKey:  []byte("ssh-ed25519 AAAAtargethostkey agent\n"),
		Send:     func(m *devopsv1.AgentMessage) error { sent <- m; return nil },
	}
	prepare := &devopsv1.PrepareSsh{SessionId: "s", PublicKey: []byte("ssh-ed25519 AAAAclientkey client"), ExpiresUnixMs: time.Now().Add(time.Minute).UnixMilli(), BindingToken: []byte("binding")}
	if err := h.Prepare(context.Background(), prepare); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-sent:
		ready := message.GetSshReady()
		if ready == nil {
			t.Fatal("no SshReady sent")
		}
		if string(ready.HostKey) != "ssh-ed25519 AAAAtargethostkey agent\n" {
			t.Fatalf("host key=%q", ready.HostKey)
		}
		if ready.SessionId != "s" {
			t.Fatalf("unexpected ready %+v", ready)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SshReady not sent")
	}
	_ = h.Close("s")
}
func TestHandlerRejectsExpiredDuplicateAndWrongTarget(t *testing.T) {
	h := agent.Handler{AgentID: "a", Dispatcher: maintenance.Dispatcher{Runner: maintenance.Runner{MaxOutputBytes: 10}}, Send: func(*devopsv1.AgentMessage) error { return nil }}
	base := &devopsv1.Job{Id: "j", AgentId: "a", DeadlineUnixMs: time.Now().Add(time.Minute).UnixMilli(), Attempt: 1, DispatchId: "d"}
	wrong := proto.Clone(base).(*devopsv1.Job)
	wrong.AgentId = "b"
	if h.Handle(context.Background(), &devopsv1.ValidatorMessage{Payload: &devopsv1.ValidatorMessage_Job{Job: wrong}}) == nil {
		t.Fatal("wrong target accepted")
	}
	expired := proto.Clone(base).(*devopsv1.Job)
	expired.DeadlineUnixMs = time.Now().Add(-time.Second).UnixMilli()
	if h.Handle(context.Background(), &devopsv1.ValidatorMessage{Payload: &devopsv1.ValidatorMessage_Job{Job: expired}}) == nil {
		t.Fatal("expired accepted")
	}
	h.AgentID = ""
	if err := h.Handle(context.Background(), &devopsv1.ValidatorMessage{Payload: &devopsv1.ValidatorMessage_Job{Job: base}}); err != nil {
		t.Fatal(err)
	}
	if h.Handle(context.Background(), &devopsv1.ValidatorMessage{Payload: &devopsv1.ValidatorMessage_Job{Job: base}}) == nil {
		t.Fatal("duplicate accepted")
	}
}
