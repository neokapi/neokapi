package server

import (
	"context"
	"testing"
	"time"

	"github.com/gokapi/gokapi/bowrain/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const testJWTSecret = "test-secret-key-for-grpc-auth"

func TestExtractClaimsValid(t *testing.T) {
	user := &auth.User{ID: "user-1", Email: "alice@test.com", Name: "Alice"}
	token, err := auth.GenerateToken(user, testJWTSecret, time.Hour)
	require.NoError(t, err)

	md := metadata.Pairs("authorization", "Bearer "+token)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	claims, err := extractClaims(ctx, testJWTSecret)
	require.NoError(t, err)
	assert.Equal(t, "user-1", claims.Subject)
	assert.Equal(t, "alice@test.com", claims.Email)
	assert.Equal(t, "Alice", claims.Name)
}

func TestExtractClaimsMissingMetadata(t *testing.T) {
	ctx := context.Background()
	_, err := extractClaims(ctx, testJWTSecret)
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	assert.Contains(t, err.Error(), "missing metadata")
}

func TestExtractClaimsMissingAuthHeader(t *testing.T) {
	md := metadata.Pairs("other-key", "value")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	_, err := extractClaims(ctx, testJWTSecret)
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	assert.Contains(t, err.Error(), "missing authorization header")
}

func TestExtractClaimsInvalidFormat(t *testing.T) {
	md := metadata.Pairs("authorization", "NotBearer abc123")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	_, err := extractClaims(ctx, testJWTSecret)
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	assert.Contains(t, err.Error(), "invalid authorization header format")
}

func TestExtractClaimsInvalidToken(t *testing.T) {
	md := metadata.Pairs("authorization", "Bearer invalid-jwt-token")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	_, err := extractClaims(ctx, testJWTSecret)
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	assert.Contains(t, err.Error(), "invalid token")
}

func TestExtractClaimsWrongSecret(t *testing.T) {
	user := &auth.User{ID: "user-1", Email: "alice@test.com", Name: "Alice"}
	token, err := auth.GenerateToken(user, "correct-secret", time.Hour)
	require.NoError(t, err)

	md := metadata.Pairs("authorization", "Bearer "+token)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	_, err = extractClaims(ctx, "wrong-secret")
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestExtractClaimsExpired(t *testing.T) {
	user := &auth.User{ID: "user-1", Email: "alice@test.com", Name: "Alice"}
	// Generate a token that expired 1 hour ago.
	token, err := auth.GenerateToken(user, testJWTSecret, -time.Hour)
	require.NoError(t, err)

	md := metadata.Pairs("authorization", "Bearer "+token)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	_, err = extractClaims(ctx, testJWTSecret)
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestGRPCUserFromContext(t *testing.T) {
	claims := &auth.Claims{Email: "alice@test.com", Name: "Alice"}
	ctx := context.WithValue(context.Background(), grpcUserKey{}, claims)

	got, ok := GRPCUserFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, "alice@test.com", got.Email)
}

func TestGRPCUserFromContextMissing(t *testing.T) {
	_, ok := GRPCUserFromContext(context.Background())
	assert.False(t, ok)
}

func TestUnaryInterceptorRejectsUnauthenticated(t *testing.T) {
	interceptor := GRPCAuthUnaryInterceptor(testJWTSecret)

	handler := func(ctx context.Context, req any) (any, error) {
		t.Fatal("handler should not be called")
		return nil, nil
	}

	_, err := interceptor(context.Background(), nil, nil, handler)
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestUnaryInterceptorPassesAuthenticated(t *testing.T) {
	interceptor := GRPCAuthUnaryInterceptor(testJWTSecret)

	user := &auth.User{ID: "user-1", Email: "alice@test.com", Name: "Alice"}
	token, err := auth.GenerateToken(user, testJWTSecret, time.Hour)
	require.NoError(t, err)

	md := metadata.Pairs("authorization", "Bearer "+token)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handlerCalled := false
	handler := func(ctx context.Context, req any) (any, error) {
		handlerCalled = true
		claims, ok := GRPCUserFromContext(ctx)
		require.True(t, ok)
		assert.Equal(t, "user-1", claims.Subject)
		return "ok", nil
	}

	resp, err := interceptor(ctx, nil, nil, handler)
	require.NoError(t, err)
	assert.True(t, handlerCalled)
	assert.Equal(t, "ok", resp)
}
