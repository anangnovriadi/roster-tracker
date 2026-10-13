package postgres

import (
	"fmt"
	"strings"

	"github.com/anangnovriadi/roster-tracker/store"
)

// postgresNoteAccessPredicate builds the canonical note-local read predicate
// for noteAlias and appends its bind values to args. Callers supply distinct
// aliases when the predicate is embedded more than once in a query.
func postgresNoteAccessPredicate(access *store.NoteAccessScope, noteAlias, memberAlias string, args *[]any) string {
	clauses := []string{}
	if access.AllowPublic {
		clauses = append(clauses, noteAlias+".visibility = 'PUBLIC'")
	}
	if access.UserID != nil {
		activeHolder := placeholder(len(*args) + 1)
		*args = append(*args, *access.UserID)
		authenticatedClauses := []string{}

		privateHolder := placeholder(len(*args) + 1)
		*args = append(*args, *access.UserID)
		authenticatedClauses = append(authenticatedClauses, "("+noteAlias+".visibility = 'PRIVATE' AND "+noteAlias+".creator_id = "+privateHolder+")")
		if access.AllowProtected {
			authenticatedClauses = append(authenticatedClauses, noteAlias+".visibility = 'PROTECTED'")
		}
		memberHolder := placeholder(len(*args) + 1)
		*args = append(*args, *access.UserID)
		authenticatedClauses = append(authenticatedClauses, "("+noteAlias+".visibility = 'SPACE' AND EXISTS (SELECT 1 FROM space_member AS "+memberAlias+" WHERE "+memberAlias+".space_id = "+noteAlias+".space_id AND "+memberAlias+".user_id = "+memberHolder+" AND "+memberAlias+".status = 'ACTIVE' AND "+memberAlias+".role IN ('ADMIN', 'USER')))")

		clauses = append(clauses, `(EXISTS (SELECT 1 FROM "user" AS access_user WHERE access_user.id = `+activeHolder+` AND access_user.row_status = 'NORMAL') AND (`+strings.Join(authenticatedClauses, " OR ")+`))`)
	}
	if len(clauses) == 0 {
		return "1 = 0"
	}

	validNote := "(" + noteAlias + ".visibility IN ('PUBLIC', 'PROTECTED', 'PRIVATE', 'SPACE')" +
		` AND EXISTS (SELECT 1 FROM "user" AS valid_creator WHERE valid_creator.id = ` + noteAlias + `.creator_id AND valid_creator.row_status IN ('NORMAL', 'ARCHIVED'))` +
		" AND (" + noteAlias + ".visibility <> 'SPACE' OR (" + noteAlias + ".space_id IS NOT NULL" +
		" AND EXISTS (SELECT 1 FROM space AS valid_space WHERE valid_space.id = " + noteAlias + ".space_id))))"
	validState := noteAlias + ".row_status = 'NORMAL'"
	if access.UserID != nil {
		targetStateHolder := placeholder(len(*args) + 1)
		*args = append(*args, *access.UserID)
		validState = fmt.Sprintf("(%s.row_status = 'NORMAL' OR (%s.row_status = 'ARCHIVED' AND %s.creator_id = %s))", noteAlias, noteAlias, noteAlias, targetStateHolder)
	}
	return "(" + strings.Join(clauses, " OR ") + ") AND " + validNote + " AND " + validState
}
