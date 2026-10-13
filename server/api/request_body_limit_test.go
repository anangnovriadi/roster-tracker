package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Only procedures that carry file bytes inline keep the large body cap; every
// other request is bounded by the general cap so buffered bodies cannot be
// used to exhaust memory.
func TestRequestBodyLimitPerProcedure(t *testing.T) {
	require.EqualValues(t, uploadRequestLimit, requestBodyLimit(attachmentUploadProcedure))
	require.EqualValues(t, uploadRequestLimit, requestBodyLimit(importNotesProcedure))
	require.EqualValues(t, MaxAPIRequestBytes, requestBodyLimit("/roster.api.AttachmentService/CreateAttachment"))
	require.EqualValues(t, MaxAPIRequestBytes, requestBodyLimit("/roster.api.AIService/Transcribe"))
	for _, procedure := range []string{
		"/roster.api.AuthService/SignIn",
		"/roster.api.UserService/CreateUser",
		"/roster.api.NoteService/CreateNote",
		"/roster.api.NoteService/ListNotes",
	} {
		require.EqualValues(t, maxGeneralRequestBytes, requestBodyLimit(procedure), procedure)
	}
	require.Less(t, int64(maxGeneralRequestBytes), int64(MaxAPIRequestBytes))
}
