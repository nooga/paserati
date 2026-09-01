package errors

// PositionedError carries a PaseratiError - and therefore its structured
// Position, including the Source file it came from - across an API boundary
// that only speaks the standard `error` interface.
//
// It exists because stringifying a diagnostic with
// fmt.Errorf("<prefix>: %s", diag.Error()) is lossy in a way that isn't
// visible until much later: the message keeps the real "line:column" as text,
// but the structured position - the only thing errors.DisplayErrors can
// actually render a caret and a source snippet against - is gone. A consumer
// that then rebuilds an error from the string has no choice but to invent a
// position, and the snippet it prints comes from whatever unrelated file
// happened to be the display fallback (#148: a module that failed to compile
// reported its own correct 4:11 in the message while underlining line 1 of the
// entry script).
//
// The rendered message is unchanged from the fmt.Errorf form it replaces, so
// this is safe to introduce anywhere a caller only ever reads .Error().
// Consumers that want the real location type-assert for Positioned.
type PositionedError struct {
	Prefix string        // e.g. "parsing failed", "compilation failed"
	Diag   PaseratiError // the diagnostic whose position must survive
}

// Positioned is the minimal contract a consumer needs to recover a real
// position from an opaque error. PaseratiError itself satisfies it, so a
// consumer can assert for this one interface and accept either a bare
// diagnostic or a PositionedError wrapping one.
type Positioned interface {
	error
	Pos() Position
}

// NewPositionedError wraps diag, keeping prefix as the message's leading
// context. Returns a nil error (not a typed-nil *PositionedError, which would
// read as non-nil once assigned to an error field) if diag is nil, so call
// sites can stay unconditional.
func NewPositionedError(prefix string, diag PaseratiError) error {
	if diag == nil {
		return nil
	}
	return &PositionedError{Prefix: prefix, Diag: diag}
}

func (e *PositionedError) Error() string {
	if e.Prefix == "" {
		return e.Diag.Error()
	}
	return e.Prefix + ": " + e.Diag.Error()
}

// Pos returns the wrapped diagnostic's real position, Source included.
func (e *PositionedError) Pos() Position { return e.Diag.Pos() }

// Unwrap exposes the diagnostic to errors.Is/errors.As.
func (e *PositionedError) Unwrap() error { return e.Diag }

// PositionOf recovers a real source position from an arbitrary error,
// unwrapping as far as needed. ok is false when nothing in the chain carries
// one, or when the one it carries has no line information to render.
func PositionOf(err error) (Position, bool) {
	for err != nil {
		if p, isPositioned := err.(Positioned); isPositioned {
			pos := p.Pos()
			if pos.Line > 0 {
				return pos, true
			}
		}
		u, hasUnwrap := err.(interface{ Unwrap() error })
		if !hasUnwrap {
			break
		}
		err = u.Unwrap()
	}
	return Position{}, false
}
