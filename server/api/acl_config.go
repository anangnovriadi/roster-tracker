package api

// PublicMethods defines API endpoints that don't require authentication.
// All other endpoints require a valid session or access token.
//
// This is the SINGLE SOURCE OF TRUTH for public endpoints.
// Both Connect interceptor and gRPC-Gateway interceptor use this map.
//
// Format: Full gRPC procedure path as returned by req.Spec().Procedure (Connect)
// or info.FullMethod (gRPC interceptor).
var PublicMethods = map[string]struct{}{
	// Auth Service - login/token endpoints must be accessible without auth
	"/roster.api.AuthService/SignIn":       {},
	"/roster.api.AuthService/RefreshToken": {}, // Token refresh uses cookie, must be accessible when access token expired

	// Instance Service - needed before login to show instance info
	"/roster.api.InstanceService/GetInstanceProfile":       {},
	"/roster.api.InstanceService/GetInstanceSetting":       {},
	"/roster.api.InstanceService/BatchGetInstanceSettings": {},

	// User Service - public user profiles and stats
	"/roster.api.UserService/CreateUser":    {}, // Registration policy is enforced in UserService
	"/roster.api.UserService/GetUser":       {},
	"/roster.api.UserService/BatchGetUsers": {},
	"/roster.api.UserService/GetUserStats":  {},
	"/roster.api.UserService/ListUserStats": {},

	// Identity Provider Service - SSO buttons on login page
	"/roster.api.IdentityProviderService/ListIdentityProviders": {},

	// Note Service - public notes (visibility filtering done in service layer)
	"/roster.api.NoteService/GetNote":              {},
	"/roster.api.NoteService/ListNotes":            {},
	"/roster.api.NoteService/ListNoteComments":     {},
	"/roster.api.NoteService/ListNoteAttachments":  {},
	"/roster.api.NoteService/ListNoteReactions":    {},
	"/roster.api.NoteService/ListNoteRelations":    {},
	"/roster.api.NoteService/GetLinkMetadata":      {},
	"/roster.api.NoteService/BatchGetLinkMetadata": {},

	// Attachment metadata follows the visibility of its linked note.
	"/roster.api.AttachmentService/GetAttachment": {},

	// Note sharing - share-token endpoints require no authentication
	"/roster.api.NoteService/GetSharedNote": {},

	// Roster Service - signed-out visitors can see who is on duty
	"/roster.api.RosterService/ListRosterAssignments": {},
}

// IsPublicMethod checks if a procedure path is public (no authentication required).
// Returns true for public methods, false for protected methods.
func IsPublicMethod(procedure string) bool {
	_, ok := PublicMethods[procedure]
	return ok
}

// AuthBootstrapMethods is the subset of PublicMethods that stays reachable by
// anonymous callers even when the instance access mode is PRIVATE.
//
// It is the minimum required to render the sign-in page, authenticate, and follow
// share links, and register when instance settings permit it. Every entry here
// MUST also exist in PublicMethods.
var AuthBootstrapMethods = map[string]struct{}{
	// Auth Service - sign-in and token refresh.
	"/roster.api.AuthService/SignIn":       {},
	"/roster.api.AuthService/RefreshToken": {},

	// Instance Service - needed to render the sign-in page (branding, auth options).
	"/roster.api.InstanceService/GetInstanceProfile":       {},
	"/roster.api.InstanceService/GetInstanceSetting":       {},
	"/roster.api.InstanceService/BatchGetInstanceSettings": {},

	// Identity Provider Service - SSO buttons on the sign-in page.
	"/roster.api.IdentityProviderService/ListIdentityProviders": {},

	// User Service - CreateUser applies registration and password-auth settings.
	"/roster.api.UserService/CreateUser": {},

	// Note sharing - share-token access stays public even on a private instance.
	"/roster.api.NoteService/GetSharedNote": {},
}

// IsAuthBootstrapMethod reports whether an anonymous request to procedure is one
// of the fixed endpoints allowed while the instance is private.
func IsAuthBootstrapMethod(procedure string) bool {
	_, ok := AuthBootstrapMethods[procedure]
	return ok
}
