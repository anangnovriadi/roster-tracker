// Package noteexport implements the Roster Tracker Export Format: a ZIP container that
// carries one user's notes and attachments between Roster Tracker instances.
//
// The package owns the container layout, the JSON records, and the reading
// and writing rules defined in docs/design/notes-export-format.md. It has no
// opinion about how records map onto a Roster Tracker instance; transports call the
// Writer with data they have already loaded and interpret the records a
// Reader returns.
package noteexport
