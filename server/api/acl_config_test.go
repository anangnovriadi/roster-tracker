package api

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	// Registers the API service descriptors looked up below.
	_ "github.com/anangnovriadi/roster-tracker/proto/gen/api"
)

// TestPublicMethodsArePublic verifies that methods in PublicMethods are recognized as public.
func TestPublicMethodsArePublic(t *testing.T) {
	publicMethods := []string{
		// Auth Service
		"/roster.api.AuthService/SignIn",
		"/roster.api.AuthService/RefreshToken",
		// Instance Service
		"/roster.api.InstanceService/GetInstanceProfile",
		"/roster.api.InstanceService/GetInstanceSetting",
		"/roster.api.InstanceService/BatchGetInstanceSettings",
		// User Service
		"/roster.api.UserService/CreateUser",
		"/roster.api.UserService/GetUser",
		"/roster.api.UserService/BatchGetUsers",
		"/roster.api.UserService/GetUserStats",
		"/roster.api.UserService/ListUserStats",
		// Identity Provider Service
		"/roster.api.IdentityProviderService/ListIdentityProviders",
		// Note Service
		"/roster.api.NoteService/GetNote",
		"/roster.api.NoteService/ListNotes",
		"/roster.api.NoteService/ListNoteComments",
		"/roster.api.NoteService/ListNoteAttachments",
		"/roster.api.NoteService/ListNoteReactions",
		"/roster.api.NoteService/ListNoteRelations",
		"/roster.api.NoteService/GetLinkMetadata",
		"/roster.api.NoteService/BatchGetLinkMetadata",
		// Attachment Service metadata follows linked note visibility.
		"/roster.api.AttachmentService/GetAttachment",
	}

	for _, method := range publicMethods {
		t.Run(method, func(t *testing.T) {
			assert.True(t, IsPublicMethod(method), "Expected %s to be public", method)
		})
	}
}

// TestProtectedMethodsRequireAuth verifies that non-public methods are recognized as protected.
func TestProtectedMethodsRequireAuth(t *testing.T) {
	protectedMethods := []string{
		"/roster.api.AttachmentService/UploadAttachment",
		// Auth Service - logout and get current user require auth
		"/roster.api.AuthService/SignOut",
		"/roster.api.AuthService/GetCurrentUser",
		// Instance Service - admin operations
		"/roster.api.InstanceService/UpdateInstanceSetting",
		"/roster.api.InstanceService/TestInstanceEmailSetting",
		// User Service - modification operations
		"/roster.api.UserService/ListUsers",
		"/roster.api.UserService/UpdateUser",
		"/roster.api.UserService/DeleteUser",
		// Note Service - write operations
		"/roster.api.NoteService/CreateNote",
		"/roster.api.NoteService/UpdateNote",
		"/roster.api.NoteService/DeleteNote",
		// Space Service - every operation requires an authenticated member.
		"/roster.api.SpaceService/CreateSpace",
		"/roster.api.SpaceService/ListSpaces",
		"/roster.api.SpaceService/GetSpace",
		"/roster.api.SpaceService/UpdateSpace",
		"/roster.api.SpaceService/DeleteSpace",
		"/roster.api.SpaceService/CreateSpaceInvitation",
		"/roster.api.SpaceService/ListSpaceInvitations",
		"/roster.api.SpaceService/ListUserSpaceInvitations",
		"/roster.api.SpaceService/GetSpaceInvitation",
		"/roster.api.SpaceService/DeleteSpaceInvitation",
		"/roster.api.SpaceService/AcceptSpaceInvitation",
		"/roster.api.SpaceService/DeclineSpaceInvitation",
		"/roster.api.SpaceService/ListSpaceMembers",
		"/roster.api.SpaceService/GetSpaceMember",
		"/roster.api.SpaceService/UpdateSpaceMember",
		"/roster.api.SpaceService/DeleteSpaceMember",
		// Attachment Service - write operations
		"/roster.api.AttachmentService/CreateAttachment",
		"/roster.api.AttachmentService/DeleteAttachment",
		// User Service - saved note views
		"/roster.api.UserService/CreateNoteView",
		"/roster.api.UserService/GetNoteView",
		"/roster.api.UserService/ListNoteViews",
		"/roster.api.UserService/UpdateNoteView",
		"/roster.api.UserService/DeleteNoteView",
	}

	for _, method := range protectedMethods {
		t.Run(method, func(t *testing.T) {
			assert.False(t, IsPublicMethod(method), "Expected %s to require auth", method)
		})
	}
}

// TestUnknownMethodsRequireAuth verifies that unknown methods default to requiring auth.
func TestUnknownMethodsRequireAuth(t *testing.T) {
	unknownMethods := []string{
		"/unknown.Service/Method",
		"/roster.api.UnknownService/Method",
		"",
		"invalid",
	}

	for _, method := range unknownMethods {
		t.Run(method, func(t *testing.T) {
			assert.False(t, IsPublicMethod(method), "Unknown method %q should require auth", method)
		})
	}
}

// TestAuthBootstrapMethodsAreSubsetOfPublic verifies every auth-bootstrap method is
// also a public method. A bootstrap method that wasn't public would be rejected before
// the private-instance check runs, breaking sign-in on a private instance.
func TestAuthBootstrapMethodsAreSubsetOfPublic(t *testing.T) {
	for method := range AuthBootstrapMethods {
		t.Run(method, func(t *testing.T) {
			assert.True(t, IsPublicMethod(method), "auth-bootstrap method %s must also be a public method", method)
		})
	}
}

// TestAuthBootstrapClassification verifies which endpoints remain reachable by
// anonymous callers when the instance access mode is PRIVATE.
func TestAuthBootstrapClassification(t *testing.T) {
	// Reachable while private: sign-in flow, registration, instance metadata, SSO, share links.
	bootstrap := []string{
		"/roster.api.AuthService/SignIn",
		"/roster.api.AuthService/RefreshToken",
		"/roster.api.UserService/CreateUser",
		"/roster.api.InstanceService/GetInstanceProfile",
		"/roster.api.InstanceService/GetInstanceSetting",
		"/roster.api.InstanceService/BatchGetInstanceSettings",
		"/roster.api.IdentityProviderService/ListIdentityProviders",
		"/roster.api.NoteService/GetSharedNote",
	}
	for _, method := range bootstrap {
		t.Run("bootstrap/"+method, func(t *testing.T) {
			assert.True(t, IsAuthBootstrapMethod(method), "expected %s to be reachable on a private instance", method)
		})
	}

	// Public in PUBLIC mode, but gated in PRIVATE mode: browsing and profiles.
	gatedWhilePrivate := []string{
		"/roster.api.NoteService/ListNotes",
		"/roster.api.NoteService/GetNote",
		"/roster.api.NoteService/ListNoteComments",
		"/roster.api.NoteService/ListNoteAttachments",
		"/roster.api.NoteService/ListNoteReactions",
		"/roster.api.NoteService/ListNoteRelations",
		"/roster.api.AttachmentService/GetAttachment",
		"/roster.api.UserService/GetUser",
		"/roster.api.UserService/ListUserStats",
	}
	for _, method := range gatedWhilePrivate {
		t.Run("gated/"+method, func(t *testing.T) {
			assert.False(t, IsAuthBootstrapMethod(method), "expected %s to be gated on a private instance", method)
		})
	}
}

// TestACLMethodsExist verifies every ACL entry names a real RPC, so a renamed
// or removed method cannot leave a stale public entry behind.
func TestACLMethodsExist(t *testing.T) {
	for _, methods := range []map[string]struct{}{PublicMethods, AuthBootstrapMethods} {
		for method := range methods {
			t.Run(method, func(t *testing.T) {
				name := protoreflect.FullName(strings.ReplaceAll(strings.TrimPrefix(method, "/"), "/", "."))
				descriptor, err := protoregistry.GlobalFiles.FindDescriptorByName(name)
				if assert.NoError(t, err, "%s does not name a registered RPC", method) {
					_, ok := descriptor.(protoreflect.MethodDescriptor)
					assert.True(t, ok, "%s is not an RPC method", method)
				}
			})
		}
	}
}
