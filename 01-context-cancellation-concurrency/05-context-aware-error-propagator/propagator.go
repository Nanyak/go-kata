package propagator

import (
	"context"
	"errors"
	"fmt"
)

// ============================================================================
// Custom Error Types
// ============================================================================

// AuthError represents authentication-related errors
type AuthError struct {
	Op        string // Operation that failed (e.g., "validate_token", "refresh_token")
	UserID    string // User identifier (safe for logging)
	APIKey    string // API key (must be redacted in Error())
	Err       error  // Underlying error
	isTimeout bool
	isTemp    bool
}

func (e *AuthError) Error() string {
	// TODO: Implement error message that redacts sensitive info (APIKey)
	redactedKey := "[REDACTED]"
	if e.APIKey != "" {
		redactedKey = ""
	}
	return fmt.Errorf("auth error during %s for user %s (key=%s): %w", e.Op, e.UserID, redactedKey, e.Err).Error()
}

func (e *AuthError) Unwrap() error {
	// TODO: Return the wrapped error for errors.Is/As support
	return e.Err
}

func (e *AuthError) Timeout() bool {
	// TODO: Return whether this error is a timeout
	return e.isTimeout
}

func (e *AuthError) Temporary() bool {
	// TODO: Return whether this error is temporary/retriable
	return e.isTemp
}

// MetadataError represents metadata service errors (database operations)
type MetadataError struct {
	Op     string // Operation (e.g., "insert", "update", "query")
	FileID string // File identifier
	Err    error  // Underlying error
	isTemp bool
}

func (e *MetadataError) Error() string {
	// TODO: Implement error message
	return fmt.Errorf("metadata error during %s for file %s: %w", e.Op, e.FileID, e.Err).Error()
}

func (e *MetadataError) Unwrap() error {
	// TODO: Return the wrapped error
	return e.Err
}

func (e *MetadataError) Temporary() bool {
	// TODO: Return whether this error is temporary (e.g., deadlock)
	return e.isTemp
}

// StorageError represents blob storage errors
type StorageError struct {
	Op        string // Operation (e.g., "upload", "download", "delete")
	Bucket    string // Storage bucket
	Key       string // Object key
	Err       error  // Underlying error
	isTimeout bool
	isTemp    bool
}

func (e *StorageError) Error() string {
	// TODO: Implement error message
	return fmt.Errorf("storage error during %s for bucket %s and key %s: %w", e.Op, e.Bucket, e.Key, e.Err).Error()
}

func (e *StorageError) Unwrap() error {
	// TODO: Return the wrapped error
	return e.Err
}

func (e *StorageError) Timeout() bool {
	// TODO: Return whether this error is a timeout
	return e.isTimeout
}

func (e *StorageError) Temporary() bool {
	// TODO: Return whether this error is temporary
	return e.isTemp
}

// StorageQuotaError represents quota exceeded errors
type StorageQuotaError struct {
	Bucket       string
	CurrentUsage int64
	Limit        int64
	Err          error
}

func (e *StorageQuotaError) Error() string {
	// TODO: Implement error message showing quota details
	return fmt.Errorf("storage quota exceeded for bucket %s: usage %d / limit %d: %w", e.Bucket, e.CurrentUsage, e.Limit, e.Err).Error()
}

func (e *StorageQuotaError) Unwrap() error {
	// TODO: Return the wrapped error
	return e.Err
}

// ============================================================================
// Service Interfaces
// ============================================================================

// AuthService handles authentication and authorization
type AuthService interface {
	// ValidateToken validates the provided token and returns the user ID
	// Returns AuthError on failure
	ValidateToken(ctx context.Context, token string) (userID string, err error)
}

// MetadataService handles file metadata operations
type MetadataService interface {
	// CreateFileRecord creates a new file metadata entry
	// Returns MetadataError on failure
	CreateFileRecord(ctx context.Context, userID, fileName string, size int64) (fileID string, err error)

	// UpdateFileStatus updates the file upload status
	// Returns MetadataError on failure
	UpdateFileStatus(ctx context.Context, fileID, status string) error
}

// StorageService handles blob storage operations
type StorageService interface {
	// UploadFile uploads file content to storage
	// Returns StorageError or StorageQuotaError on failure
	UploadFile(ctx context.Context, bucket, key string, data []byte) error
}

// ============================================================================
// Gateway Implementation
// ============================================================================

// FileUploadRequest represents a file upload request
type FileUploadRequest struct {
	Token    string
	FileName string
	Bucket   string
	Data     []byte
}

// CloudStorageGateway coordinates file uploads across services
type CloudStorageGateway struct {
	auth     AuthService
	metadata MetadataService
	storage  StorageService
}

// NewCloudStorageGateway creates a new gateway with the provided services
func NewCloudStorageGateway(auth AuthService, metadata MetadataService, storage StorageService) *CloudStorageGateway {
	return &CloudStorageGateway{
		auth:     auth,
		metadata: metadata,
		storage:  storage,
	}
}

// UploadFile handles the complete file upload flow
// It validates auth, creates metadata, and uploads to storage
// Errors are wrapped with context at each layer
func (g *CloudStorageGateway) UploadFile(ctx context.Context, req FileUploadRequest) error {
	// 1. Validate token
	userID, err := g.auth.ValidateToken(ctx, req.Token)
	if err != nil {
		return WrapWithContext(err, "upload failed: auth")
	}

	// 2. Create file record
	fileID, err := g.metadata.CreateFileRecord(ctx, userID, req.FileName, int64(len(req.Data)))
	if err != nil {
		return WrapWithContext(err, "create file record failed")
	}

	// 3. Upload to storage
	err = g.storage.UploadFile(ctx, req.Bucket, fileID, req.Data)
	if err != nil {
		// Update status to "failed" before returning
		_ = g.metadata.UpdateFileStatus(ctx, fileID, "failed")
		return WrapWithContext(err, "upload failed: storage")
	}

	// 4. Update status on success
	if err := g.metadata.UpdateFileStatus(ctx, fileID, "completed"); err != nil {
		return WrapWithContext(err, "upload failed: status update")
	}

	return nil
}

// ============================================================================
// Helper Functions
// ============================================================================

// IsTimeout checks if the error (or any wrapped error) is a timeout
func IsTimeout(err error) bool {
	// TODO: Check for timeout errors using errors.As
	// Should work with AuthError, StorageError, and context.DeadlineExceeded
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var te timeoutError
	if errors.As(err, &te) && te.Timeout() {
		return true
	}
	return false
}

// IsTemporary checks if the error (or any wrapped error) is temporary/retriable
func IsTemporary(err error) bool {
	// TODO: Check for temporary errors using errors.As
	if err == nil {
		return false
	}
	var te temporaryError
	if errors.As(err, &te) && te.Temporary() {
		return true
	}
	return false
}

// WrapWithContext wraps an error with additional context using fmt.Errorf and %w
func WrapWithContext(err error, format string, args ...interface{}) error {
	// TODO: Wrap error while preserving the error chain
	if err == nil {
		return nil
	}
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("%s: %w", msg, err)
}

// Sentinel errors for common failure cases
var (
	ErrAuthFailed         = errors.New("authentication failed")
	ErrTokenExpired       = errors.New("token expired")
	ErrInvalidToken       = errors.New("invalid token")
	ErrDatabaseDeadlock   = errors.New("database deadlock")
	ErrStorageUnavailable = errors.New("storage service unavailable")
	ErrQuotaExceeded      = errors.New("storage quota exceeded")
)

// timeoutError interface for checking timeout errors
type timeoutError interface {
	Timeout() bool
}

// temporaryError interface for checking temporary errors
type temporaryError interface {
	Temporary() bool
}

// Ensure variables are used to avoid compiler errors
var (
	_ = context.Background
	_ = fmt.Errorf
)
