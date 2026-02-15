package server

import (
	"context"
	"strings"

	"github.com/gokapi/gokapi/core/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type grpcUserKey struct{}

// GRPCAuthUnaryInterceptor validates JWT tokens from gRPC metadata and injects
// the authenticated user claims into the context.
func GRPCAuthUnaryInterceptor(jwtSecret string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		claims, err := extractClaims(ctx, jwtSecret)
		if err != nil {
			return nil, err
		}
		ctx = context.WithValue(ctx, grpcUserKey{}, claims)
		return handler(ctx, req)
	}
}

// GRPCAuthStreamInterceptor validates JWT tokens for streaming RPCs.
func GRPCAuthStreamInterceptor(jwtSecret string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		claims, err := extractClaims(ss.Context(), jwtSecret)
		if err != nil {
			return err
		}
		wrapped := &authServerStream{
			ServerStream: ss,
			ctx:          context.WithValue(ss.Context(), grpcUserKey{}, claims),
		}
		return handler(srv, wrapped)
	}
}

// GRPCUserFromContext retrieves the authenticated user claims from a gRPC context.
func GRPCUserFromContext(ctx context.Context) (*auth.Claims, bool) {
	claims, ok := ctx.Value(grpcUserKey{}).(*auth.Claims)
	return claims, ok
}

func extractClaims(ctx context.Context, jwtSecret string) (*auth.Claims, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}

	values := md.Get("authorization")
	if len(values) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing authorization header")
	}

	token := strings.TrimPrefix(values[0], "Bearer ")
	if token == values[0] {
		return nil, status.Error(codes.Unauthenticated, "invalid authorization header format")
	}

	claims, err := auth.ValidateToken(token, jwtSecret)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
	}
	return claims, nil
}

// authServerStream wraps a grpc.ServerStream with an augmented context.
type authServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *authServerStream) Context() context.Context {
	return s.ctx
}
