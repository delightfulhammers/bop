package platform

// DeviceFlowResponse is the response from POST /auth/device.
type DeviceFlowResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// TokenResponse is the response from POST /auth/device/token and POST /auth/refresh.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	UserID       string `json:"user_id"`
	Username     string `json:"username"`
}

// UserInfoResponse is the response from GET /auth/me.
type UserInfoResponse struct {
	UserID       string   `json:"user_id"`
	IdentityID   string   `json:"identity_id,omitempty"`
	Email        string   `json:"email,omitempty"`
	Username     string   `json:"username,omitempty"`
	DisplayName  string   `json:"display_name,omitempty"`
	AvatarURL    string   `json:"avatar_url,omitempty"`
	Role         string   `json:"role"`
	IsActive     bool     `json:"is_active"`
	ProductID    string   `json:"product_id"`
	TenantID     string   `json:"tenant_id"`
	ProviderType string   `json:"provider_type,omitempty"`
	ProviderID   string   `json:"provider_id,omitempty"`
	OrgID        string   `json:"org_id,omitempty"`
	OrgSlug      string   `json:"org_slug,omitempty"`
	OrgRole      string   `json:"org_role,omitempty"`
	PlanID       string   `json:"plan_id"`
	PlanStatus   string   `json:"plan_status"`
	Entitlements []string `json:"entitlements"`
}

// TeamResponse represents a single team in the GET /v1/teams response.
type TeamResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Tier      string `json:"tier"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
}

// ListTeamsResponse is the response from GET /v1/teams.
type ListTeamsResponse struct {
	Teams []TeamResponse `json:"teams"`
}

// ConfigResponse is the response from GET /products/{product_id}/config.
type ConfigResponse struct {
	Config         map[string]any `json:"config"`
	EditableFields []string       `json:"editable_fields"`
	Tier           string         `json:"tier"`
	IsReadOnly     bool           `json:"is_read_only"`
	Version        int            `json:"version"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// DeviceFlowError is returned when the device flow polling gets an expected error.
type DeviceFlowError struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}
