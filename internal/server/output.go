package server

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	devopsv1 "silent-devops/api/devops/v1"
)

func (s Fleet) StreamJobOutput(r *devopsv1.StreamJobOutputRequest, stream devopsv1.FleetService_StreamJobOutputServer) error {
	var output []byte
	var truncated bool
	var state int32
	err := s.DB.QueryRowContext(stream.Context(), "SELECT output,output_truncated,state FROM jobs WHERE id=?", r.JobId).Scan(&output, &truncated, &state)
	if err != nil {
		return status.Error(codes.NotFound, "job output not found")
	}
	if state == int32(devopsv1.JobState_JOB_STATE_DISPATCHED) || state == int32(devopsv1.JobState_JOB_STATE_RUNNING) {
		return status.Error(codes.FailedPrecondition, "job still running")
	}
	const chunk = 32 * 1024
	for sequence, start := uint32(0), 0; start < len(output); sequence, start = sequence+1, start+chunk {
		end := start + chunk
		if end > len(output) {
			end = len(output)
		}
		_ = truncated
		if err := stream.Send(&devopsv1.OutputChunk{JobId: r.JobId, Sequence: uint64(sequence), Data: output[start:end], Final: end == len(output)}); err != nil {
			return err
		}
	}
	if len(output) == 0 {
		return stream.Send(&devopsv1.OutputChunk{JobId: r.JobId, Final: true})
	}
	return nil
}
