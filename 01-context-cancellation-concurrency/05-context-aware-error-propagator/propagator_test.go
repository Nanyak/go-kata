package propagator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// ============================================================================
// Mock Services
// ============================================================================

type mockAuthService struct {
	userID string
	err    error
}

func (m *mockAuthService) ValidateToken(ctx context.Context, token string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.userID, nil
}

type mockMetadataService struct {
	fileID    string
	createErr error
	updateErr error
}

func (m *mockMetadataService) CreateFileRecord(ctx context.Context, userID, fileName string, size int64) (string, error) {
	if m.createErr != nil {
		return "", m.createErr
	}
	return m.fileID, nil
}

func (m *mockMetadataService) UpdateFileStatus(ctx context.Context, fileID, status string) error {
	return m.updateErr
}

type mockStorageService struct {
	err error
}

func (m *mockStorageService) UploadFile(ctx context.Context, bucket, key string, data []byte) error {
	return m.err
}

// ============================================================================
// Test: The "Sensitive Data Leak" (README requirement)
// ============================================================================

func TestAuthError_RedactsSensitiveData(t *testing.T) {
	secretKey := "sk-super-secret-api-key-12345"

	authErr := &AuthError{
		Op:     "validate_token",
		UserID: "user123",
		APIKey: secretKey,
		Err:    ErrAuthFailed,
	}

	errString := fmt.Sprint(authErr)

	// FAIL CONDITION: If fmt.Sprint(err) contains the API key string
	if strings.Contains(errString, secretKey) {
		t.Errorf("SENSITIVE DATA LEAK: error string contains API key\nGot: %s", errString)
	}

	// Verify the error still contains useful info
	if !strings.Contains(errString, "validate_token") {
		t.Error("error should contain operation name")
	}
	if !strings.Contains(errString, "user123") {
		t.Error("error should contain user ID")
	}
}

// ============================================================================
// Test: The "Lost Context" (README requirement)
// ============================================================================

func TestAuthError_PreservesContextThroughWrapping(t *testing.T) {
	// Wrap an AuthError three times through different layers
	authErr := &AuthError{
		Op:     "validate",
		UserID: "user456",
		Err:    ErrAuthFailed,
	}

	// Simulate wrapping through layers
	err1 := fmt.Errorf("metadata layer: %w", authErr)
	err2 := fmt.Errorf("storage layer: %w", err1)
	err3 := fmt.Errorf("gateway layer: %w", err2)

	// FAIL CONDITION: If errors.As(err, &AuthError{}) returns false
	var extractedErr *AuthError
	if !errors.As(err3, &extractedErr) {
		t.Error("LOST CONTEXT: errors.As failed to find AuthError through wrapped layers")
	}

	// Verify we can still access the original error's fields
	if extractedErr.Op != "validate" {
		t.Errorf("expected Op='validate', got '%s'", extractedErr.Op)
	}
	if extractedErr.UserID != "user456" {
		t.Errorf("expected UserID='user456', got '%s'", extractedErr.UserID)
	}

	// Also verify errors.Is works for the sentinel error
	if !errors.Is(err3, ErrAuthFailed) {
		t.Error("errors.Is failed to find ErrAuthFailed through wrapped layers")
	}
}

// ============================================================================
// Test: The "Timeout Confusion" (README requirement)
// ============================================================================

func TestStorageError_PreservesDeadlineExceeded(t *testing.T) {
	// Create a timeout error in the storage layer
	storageErr := &StorageError{
		Op:        "upload",
		Bucket:    "my-bucket",
		Key:       "my-key",
		Err:       context.DeadlineExceeded,
		isTimeout: true,
	}

	// Wrap it through layers
	wrappedErr := fmt.Errorf("gateway: %w", storageErr)

	// FAIL CONDITION: If errors.Is(err, context.DeadlineExceeded) returns false
	if !errors.Is(wrappedErr, context.DeadlineExceeded) {
		t.Error("TIMEOUT CONFUSION: errors.Is failed to find context.DeadlineExceeded")
	}

	// Also verify IsTimeout helper works
	if !IsTimeout(wrappedErr) {
		t.Error("IsTimeout should return true for wrapped deadline exceeded error")
	}
}

// ============================================================================
// Error Type Tests
// ============================================================================

func TestAuthError_Unwrap(t *testing.T) {
	innerErr := errors.New("inner error")
	authErr := &AuthError{
		Op:  "test",
		Err: innerErr,
	}

	if authErr.Unwrap() != innerErr {
		t.Error("Unwrap should return the inner error")
	}
}

func TestAuthError_Timeout(t *testing.T) {
	authErr := &AuthError{isTimeout: true}
	if !authErr.Timeout() {
		t.Error("Timeout() should return true when isTimeout is true")
	}

	authErr2 := &AuthError{isTimeout: false}
	if authErr2.Timeout() {
		t.Error("Timeout() should return false when isTimeout is false")
	}
}

func TestAuthError_Temporary(t *testing.T) {
	authErr := &AuthError{isTemp: true}
	if !authErr.Temporary() {
		t.Error("Temporary() should return true when isTemp is true")
	}

	authErr2 := &AuthError{isTemp: false}
	if authErr2.Temporary() {
		t.Error("Temporary() should return false when isTemp is false")
	}
}

func TestMetadataError_Unwrap(t *testing.T) {
	innerErr := ErrDatabaseDeadlock
	metaErr := &MetadataError{
		Op:     "insert",
		FileID: "file123",
		Err:    innerErr,
	}

	if !errors.Is(metaErr, ErrDatabaseDeadlock) {
		t.Error("errors.Is should find ErrDatabaseDeadlock through MetadataError")
	}
}

func TestStorageError_Unwrap(t *testing.T) {
	storageErr := &StorageError{
		Op:     "upload",
		Bucket: "bucket",
		Key:    "key",
		Err:    ErrStorageUnavailable,
	}

	if !errors.Is(storageErr, ErrStorageUnavailable) {
		t.Error("errors.Is should find ErrStorageUnavailable through StorageError")
	}
}

func TestStorageQuotaError_Unwrap(t *testing.T) {
	quotaErr := &StorageQuotaError{
		Bucket:       "bucket",
		CurrentUsage: 100,
		Limit:        50,
		Err:          ErrQuotaExceeded,
	}

	if !errors.Is(quotaErr, ErrQuotaExceeded) {
		t.Error("errors.Is should find ErrQuotaExceeded through StorageQuotaError")
	}
}

// ============================================================================
// Helper Function Tests
// ============================================================================

func TestIsTimeout(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "context.DeadlineExceeded",
			err:      context.DeadlineExceeded,
			expected: true,
		},
		{
			name:     "wrapped DeadlineExceeded",
			err:      fmt.Errorf("wrapped: %w", context.DeadlineExceeded),
			expected: true,
		},
		{
			name:     "AuthError with timeout",
			err:      &AuthError{isTimeout: true, Err: errors.New("timeout")},
			expected: true,
		},
		{
			name:     "AuthError without timeout",
			err:      &AuthError{isTimeout: false, Err: errors.New("other")},
			expected: false,
		},
		{
			name:     "StorageError with timeout",
			err:      &StorageError{isTimeout: true, Err: errors.New("timeout")},
			expected: true,
		},
		{
			name:     "regular error",
			err:      errors.New("regular error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTimeout(tt.err); got != tt.expected {
				t.Errorf("IsTimeout() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsTemporary(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "AuthError temporary",
			err:      &AuthError{isTemp: true, Err: errors.New("temp")},
			expected: true,
		},
		{
			name:     "AuthError not temporary",
			err:      &AuthError{isTemp: false, Err: errors.New("perm")},
			expected: false,
		},
		{
			name:     "MetadataError temporary",
			err:      &MetadataError{isTemp: true, Err: ErrDatabaseDeadlock},
			expected: true,
		},
		{
			name:     "StorageError temporary",
			err:      &StorageError{isTemp: true, Err: errors.New("temp")},
			expected: true,
		},
		{
			name:     "regular error",
			err:      errors.New("regular error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTemporary(tt.err); got != tt.expected {
				t.Errorf("IsTemporary() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestWrapWithContext(t *testing.T) {
	t.Run("wraps error with context", func(t *testing.T) {
		innerErr := ErrAuthFailed
		wrapped := WrapWithContext(innerErr, "operation %s failed", "upload")

		if !errors.Is(wrapped, ErrAuthFailed) {
			t.Error("wrapped error should still match inner error with errors.Is")
		}

		if !strings.Contains(wrapped.Error(), "operation upload failed") {
			t.Errorf("wrapped error should contain context message, got: %s", wrapped.Error())
		}
	})

	t.Run("returns nil for nil error", func(t *testing.T) {
		if WrapWithContext(nil, "context") != nil {
			t.Error("WrapWithContext(nil) should return nil")
		}
	})
}

// ============================================================================
// Gateway Integration Tests
// ============================================================================

func TestCloudStorageGateway_UploadFile_Success(t *testing.T) {
	gateway := NewCloudStorageGateway(
		&mockAuthService{userID: "user123"},
		&mockMetadataService{fileID: "file456"},
		&mockStorageService{},
	)

	err := gateway.UploadFile(context.Background(), FileUploadRequest{
		Token:    "valid-token",
		FileName: "test.txt",
		Bucket:   "my-bucket",
		Data:     []byte("hello world"),
	})

	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestCloudStorageGateway_UploadFile_AuthFailure(t *testing.T) {
	authErr := &AuthError{
		Op:     "validate_token",
		UserID: "",
		Err:    ErrInvalidToken,
	}

	gateway := NewCloudStorageGateway(
		&mockAuthService{err: authErr},
		&mockMetadataService{fileID: "file456"},
		&mockStorageService{},
	)

	err := gateway.UploadFile(context.Background(), FileUploadRequest{
		Token:    "invalid-token",
		FileName: "test.txt",
		Bucket:   "my-bucket",
		Data:     []byte("hello world"),
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Should be able to find the AuthError through wrapping
	var extractedErr *AuthError
	if !errors.As(err, &extractedErr) {
		t.Error("should be able to extract AuthError from wrapped error")
	}

	// Should be able to find the sentinel error
	if !errors.Is(err, ErrInvalidToken) {
		t.Error("should be able to find ErrInvalidToken through error chain")
	}
}

func TestCloudStorageGateway_UploadFile_StorageQuotaExceeded(t *testing.T) {
	quotaErr := &StorageQuotaError{
		Bucket:       "my-bucket",
		CurrentUsage: 1000,
		Limit:        500,
		Err:          ErrQuotaExceeded,
	}

	gateway := NewCloudStorageGateway(
		&mockAuthService{userID: "user123"},
		&mockMetadataService{fileID: "file456"},
		&mockStorageService{err: quotaErr},
	)

	err := gateway.UploadFile(context.Background(), FileUploadRequest{
		Token:    "valid-token",
		FileName: "test.txt",
		Bucket:   "my-bucket",
		Data:     []byte("hello world"),
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Should be able to find StorageQuotaError
	var extractedErr *StorageQuotaError
	if !errors.As(err, &extractedErr) {
		t.Error("should be able to extract StorageQuotaError from wrapped error")
	}

	// Verify quota details are preserved
	if extractedErr.CurrentUsage != 1000 || extractedErr.Limit != 500 {
		t.Error("quota error details should be preserved")
	}

	// Should be able to find the sentinel error
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Error("should be able to find ErrQuotaExceeded through error chain")
	}
}

func TestCloudStorageGateway_UploadFile_TimeoutError(t *testing.T) {
	storageErr := &StorageError{
		Op:        "upload",
		Bucket:    "my-bucket",
		Key:       "file456",
		Err:       context.DeadlineExceeded,
		isTimeout: true,
		isTemp:    true,
	}

	gateway := NewCloudStorageGateway(
		&mockAuthService{userID: "user123"},
		&mockMetadataService{fileID: "file456"},
		&mockStorageService{err: storageErr},
	)

	err := gateway.UploadFile(context.Background(), FileUploadRequest{
		Token:    "valid-token",
		FileName: "test.txt",
		Bucket:   "my-bucket",
		Data:     []byte("hello world"),
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Should detect timeout
	if !IsTimeout(err) {
		t.Error("IsTimeout should return true for timeout error")
	}

	// Should detect temporary
	if !IsTemporary(err) {
		t.Error("IsTemporary should return true for temporary error")
	}

	// Should find context.DeadlineExceeded
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Error("should be able to find context.DeadlineExceeded through error chain")
	}
}

func TestCloudStorageGateway_UploadFile_MetadataFailure(t *testing.T) {
	metaErr := &MetadataError{
		Op:     "insert",
		FileID: "",
		Err:    ErrDatabaseDeadlock,
		isTemp: true,
	}

	gateway := NewCloudStorageGateway(
		&mockAuthService{userID: "user123"},
		&mockMetadataService{createErr: metaErr},
		&mockStorageService{},
	)

	err := gateway.UploadFile(context.Background(), FileUploadRequest{
		Token:    "valid-token",
		FileName: "test.txt",
		Bucket:   "my-bucket",
		Data:     []byte("hello world"),
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Should be able to find MetadataError
	var extractedErr *MetadataError
	if !errors.As(err, &extractedErr) {
		t.Error("should be able to extract MetadataError from wrapped error")
	}

	// Should detect temporary (deadlock is retriable)
	if !IsTemporary(err) {
		t.Error("IsTemporary should return true for deadlock error")
	}
}
