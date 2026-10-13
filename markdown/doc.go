// Package markdown is the markdown engine: parsing note content into the AST
// in markdown/ast, the notes-specific syntax in markdown/extensions and
// markdown/parser (tags, mentions, math, GFM extras), rendering, and the
// derived note payload (tags, properties, snippet).
//
// Layering: markdown may import proto/gen and internal. It must not import
// store, core, or server.
package markdown
