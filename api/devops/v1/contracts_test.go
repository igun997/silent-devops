package devopsv1_test

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	devopsv1 "silent-devops/api/devops/v1"
)

func roundTrip(t *testing.T, in proto.Message, out proto.Message) {
	t.Helper()
	b, err := proto.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if err = proto.Unmarshal(b, out); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(in, out) {
		t.Fatalf("round trip mismatch: %v", out)
	}
}

func TestSecurityCriticalContractsRoundTrip(t *testing.T) {
	job := &devopsv1.Job{Id: "j", AgentId: "a", DeadlineUnixMs: 42, Attempt: 1, DispatchId: "d", IdempotencyKey: "i", Authorization: &devopsv1.AuthorizationContext{Role: devopsv1.Role_ROLE_ADMIN, Reason: "incident", Confirmed: true}, Operation: &devopsv1.Job_UnsafeOperation{UnsafeOperation: &devopsv1.UnsafeOperation{Operation: &devopsv1.UnsafeOperation_CleanupRun{CleanupRun: &devopsv1.CleanupRunRequest{PreviewId: "p", PreviewHash: []byte("h"), PreviewExpiresUnixMs: 40}}}}}
	roundTrip(t, job, &devopsv1.Job{})
	roundTrip(t, &devopsv1.AgentMessage{Payload: &devopsv1.AgentMessage_CancelAck{CancelAck: &devopsv1.CancelAcknowledgement{JobId: "j", RequestId: "r", Outcome: devopsv1.CancelOutcome_CANCEL_OUTCOME_UNKNOWN_RESULT, TerminalState: devopsv1.JobState_JOB_STATE_UNKNOWN_RESULT}}}, &devopsv1.AgentMessage{})
	roundTrip(t, &devopsv1.EnrollRequest{Token: "secret", CsrDer: []byte{1}, ValidatorPin: "pin"}, &devopsv1.EnrollRequest{})
	roundTrip(t, &devopsv1.RenewRequest{CsrDer: []byte{1}}, &devopsv1.RenewRequest{})
	roundTrip(t, &devopsv1.PrepareSsh{SessionId: "s", PublicKey: []byte("key"), ExpiresUnixMs: 42, LoopbackPort: 22001, BindingToken: []byte("binding")}, &devopsv1.PrepareSsh{})
	roundTrip(t, &devopsv1.ValidatorMessage{Payload: &devopsv1.ValidatorMessage_PrepareSsh{PrepareSsh: &devopsv1.PrepareSsh{SessionId: "s", LoopbackPort: 22001, BindingToken: []byte("binding")}}}, &devopsv1.ValidatorMessage{})
	roundTrip(t, &devopsv1.ExecJobRequest{Context: &devopsv1.JobRequestContext{AgentId: "a", Reason: "incident", TimeoutSeconds: 30, IdempotencyKey: "i", Confirmed: true}, Request: &devopsv1.ArbitraryCommand{Command: "id"}}, &devopsv1.ExecJobRequest{})
	roundTrip(t, &devopsv1.ProtocolError{Code: devopsv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, Details: []*devopsv1.ErrorDetail{{Field: "unit", Retryable: false}}}, &devopsv1.ProtocolError{})
}

func TestUnknownFieldsSurviveRoundTrip(t *testing.T) {
	in := &devopsv1.VersionRange{Minimum: 1, Maximum: 2}
	in.ProtoReflect().SetUnknown([]byte{0x98, 0x06, 0x01})
	b, err := proto.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out devopsv1.VersionRange
	if err = proto.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.ProtoReflect().GetUnknown()) == 0 {
		t.Fatal("unknown field discarded")
	}
}

func TestStableEnumValuesAndFieldNumbers(t *testing.T) {
	if devopsv1.JobState_JOB_STATE_UNKNOWN_RESULT != 8 || devopsv1.ErrorCode_ERROR_CODE_DUPLICATE_JOB != 14 {
		t.Fatal("stable enum value changed")
	}
	field := (&devopsv1.Job{}).ProtoReflect().Descriptor().Fields().ByName(protoreflect.Name("dispatch_id"))
	if field == nil || field.Number() != 11 {
		t.Fatal("Job.dispatch_id field number changed")
	}
	if (&devopsv1.TypedOperation{}).ProtoReflect().Descriptor().Oneofs().ByName("request") == nil {
		t.Fatal("typed operation request must be oneof")
	}
}

func TestProtocolLimitsAreCompleteAndPositive(t *testing.T) {
	m := devopsv1.DefaultLimits().ProtoReflect()
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if v.Uint() == 0 {
			t.Errorf("%s is zero", fd.Name())
		}
		return true
	})
}
