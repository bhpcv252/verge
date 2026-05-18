package testhelper

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func GRPCCode(err error) codes.Code {
	s, _ := status.FromError(err)
	return s.Code()
}
