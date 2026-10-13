package noteexport

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// Warning records a recoverable deviation found while reading a record. The
// reader applies the documented fallback and continues.
type Warning struct {
	UID     string
	Message string
}

func (w Warning) String() string {
	if w.UID == "" {
		return w.Message
	}
	return w.UID + ": " + w.Message
}

var (
	knownStates       = map[string]struct{}{"NORMAL": {}, "ARCHIVED": {}}
	knownVisibilities = map[string]struct{}{"PRIVATE": {}, "PROTECTED": {}, "PUBLIC": {}, "SPACE": {}}
)

// parseFormatVersion splits MAJOR.MINOR.
func parseFormatVersion(version string) (int, int, error) {
	major, minor, ok := strings.Cut(version, ".")
	if !ok {
		return 0, 0, errors.Errorf("formatVersion %q is not MAJOR.MINOR", version)
	}
	majorNumber, err := strconv.Atoi(major)
	if err != nil || majorNumber < 0 {
		return 0, 0, errors.Errorf("formatVersion %q has an invalid major version", version)
	}
	minorNumber, err := strconv.Atoi(minor)
	if err != nil || minorNumber < 0 {
		return 0, 0, errors.Errorf("formatVersion %q has an invalid minor version", version)
	}
	return majorNumber, minorNumber, nil
}

func validateManifest(manifest *Manifest) error {
	if manifest.Format != Format && manifest.Format != legacyFormat {
		return errors.Errorf("format %q is not %q", manifest.Format, Format)
	}
	major, _, err := parseFormatVersion(manifest.FormatVersion)
	if err != nil {
		return err
	}
	if major != FormatMajor {
		return errors.Errorf("format version %s is not supported by this reader (supports %d.x)", manifest.FormatVersion, FormatMajor)
	}
	if manifest.Generator.Name == "" || manifest.Generator.Version == "" {
		return errors.New("generator name and version are required")
	}
	if _, err := ParseTime(manifest.ExportTime); err != nil {
		return errors.Wrap(err, "exportTime is invalid")
	}
	if manifest.Scope.Kind == "" {
		return errors.New("scope.kind is required")
	}
	if manifest.Scope.Kind == ScopeKindUser && (manifest.Scope.User == nil || manifest.Scope.User.Username == "") {
		return errors.New("scope.user.username is required for a USER archive")
	}
	if manifest.Counts != nil && (manifest.Counts.Notes < 0 || manifest.Counts.Attachments < 0) {
		return errors.New("counts must not be negative")
	}
	return nil
}

// validateNote checks the hard rules of a note record and applies the
// documented fallbacks for soft ones, returning a warning for each fallback.
func validateNote(note *Note) ([]Warning, error) {
	if err := ValidateUID(note.UID); err != nil {
		return nil, err
	}
	fail := func(format string, args ...any) error {
		return errors.Errorf("note %s: %s", note.UID, fmt.Sprintf(format, args...))
	}
	if note.Creator == "" {
		return nil, fail("creator is required")
	}
	if _, err := ParseTime(note.CreateTime); err != nil {
		return nil, fail("createTime is invalid: %v", err)
	}
	if _, err := ParseTime(note.UpdateTime); err != nil {
		return nil, fail("updateTime is invalid: %v", err)
	}
	if note.ContentPath == "" {
		return nil, fail("contentPath is required")
	}
	if err := ValidateEntryName(note.ContentPath); err != nil {
		return nil, fail("contentPath: %v", err)
	}
	if note.Parent != "" {
		if err := ValidateUID(note.Parent); err != nil {
			return nil, fail("parent: %v", err)
		}
		if note.Parent == note.UID {
			return nil, fail("parent must not be the note itself")
		}
	}
	if note.Space != nil {
		if err := ValidateUID(note.Space.UID); err != nil {
			return nil, fail("space: %v", err)
		}
	}
	if note.Location != nil {
		if note.Location.Latitude < -90 || note.Location.Latitude > 90 || note.Location.Longitude < -180 || note.Location.Longitude > 180 {
			return nil, fail("location is out of range")
		}
	}

	var warnings []Warning
	if _, ok := knownStates[note.State]; !ok {
		warnings = append(warnings, Warning{UID: note.UID, Message: fmt.Sprintf("unknown state %q, using NORMAL", note.State)})
		note.State = "NORMAL"
	}
	if _, ok := knownVisibilities[note.Visibility]; !ok {
		warnings = append(warnings, Warning{UID: note.UID, Message: fmt.Sprintf("unknown visibility %q, using PRIVATE", note.Visibility)})
		note.Visibility = "PRIVATE"
	}

	relations := note.Relations[:0]
	for _, relation := range note.Relations {
		if relation.Type != RelationReference {
			warnings = append(warnings, Warning{UID: note.UID, Message: fmt.Sprintf("unknown relation type %q, skipping relation", relation.Type)})
			continue
		}
		if err := ValidateUID(relation.Note); err != nil {
			return nil, fail("relation: %v", err)
		}
		relations = append(relations, relation)
	}
	note.Relations = relations

	seenAttachmentUIDs := make(map[string]struct{}, len(note.Attachments))
	for index := range note.Attachments {
		attachment := &note.Attachments[index]
		if err := ValidateUID(attachment.UID); err != nil {
			return nil, fail("attachment: %v", err)
		}
		if _, dup := seenAttachmentUIDs[attachment.UID]; dup {
			return nil, fail("attachment %s is listed twice", attachment.UID)
		}
		seenAttachmentUIDs[attachment.UID] = struct{}{}
		if attachment.Filename == "" {
			return nil, fail("attachment %s: filename is required", attachment.UID)
		}
		if !mediaTypePattern.MatchString(attachment.Type) {
			return nil, fail("attachment %s: type %q is not a media type", attachment.UID, attachment.Type)
		}
		if attachment.Size < 0 {
			return nil, fail("attachment %s: size must not be negative", attachment.UID)
		}
		if _, err := ParseTime(attachment.CreateTime); err != nil {
			return nil, fail("attachment %s: createTime is invalid: %v", attachment.UID, err)
		}
		switch {
		case attachment.Path != "" && attachment.ExternalLink != "":
			return nil, fail("attachment %s: path and externalLink are mutually exclusive", attachment.UID)
		case attachment.Path != "":
			if err := ValidateEntryName(attachment.Path); err != nil {
				return nil, fail("attachment %s: path: %v", attachment.UID, err)
			}
			if !strings.HasPrefix(attachment.Path, AttachmentsDir) {
				return nil, fail("attachment %s: path must be under %s", attachment.UID, AttachmentsDir)
			}
			if !sha256Pattern.MatchString(attachment.SHA256) {
				return nil, fail("attachment %s: sha256 is required with path", attachment.UID)
			}
		case attachment.ExternalLink != "":
		default:
			return nil, fail("attachment %s: one of path or externalLink is required", attachment.UID)
		}
	}
	for _, reaction := range note.Reactions {
		if reaction.ReactionType == "" || reaction.Creator == "" {
			return nil, fail("reaction requires reactionType and creator")
		}
	}
	return warnings, nil
}
