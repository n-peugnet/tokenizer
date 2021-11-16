package tokenizer

import (
	"strconv"
	"strings"
)

// Stream iterator via parsed tokens.
// If data reads from an infinite buffer then the iterator will be read data from reader chunk-by-chunk.
type Stream struct {
	t *Tokenizer
	// count of tokens in the stream
	len int
	// pointer to the node of double-linked list of tokens
	current *Token
	// pointer of valid token if current moved to out of bounds (out of end list)
	prev *Token
	// pointer of valid token if current moved to out of bounds (out of begin list)
	next *Token
	// pointer to head of list
	head *Token

	// last whitespaces before end of source
	wsTail []byte
	// count of parsed bytes
	parsed int

	p           *parsing
	historySize int
}

// NewStream creates new parsed stream of tokens.
func NewStream(p *parsing) *Stream {
	return &Stream{
		t:       p.t,
		head:    p.head,
		current: p.head,
		len:     p.n,
		wsTail:  p.tail,
		parsed:  p.pos + 1,
	}
}

// NewInfStream creates new stream with active parser.
func NewInfStream(p *parsing) *Stream {
	return &Stream{
		t:       p.t,
		p:       p,
		len:     p.n,
		head:    p.head,
		current: p.head,
	}
}

// SetHistorySize sets the number of tokens that should remain after the current token
func (s *Stream) SetHistorySize(size int) *Stream {
	s.historySize = size
	return s
}

// Close free all token objects to pool
func (s *Stream) Close() {
	for ptr := s.head; ptr != nil; {
		p := ptr.next
		s.t.freeToken(ptr)
		ptr = p
	}
	s.next = nil
	s.prev = nil
	s.head = undefToken
	s.current = undefToken
	s.len = 0
}

func (s *Stream) String() string {
	items := make([]string, 0, s.len)
	ptr := s.head
	for ptr != nil {
		items = append(items, strconv.Itoa(ptr.id)+": "+ptr.String())
		ptr = ptr.next
	}

	return strings.Join(items, "\n")
}

// GetParsedLength returns currently count parsed bytes.
func (s *Stream) GetParsedLength() int {
	return s.parsed
}

// Next moves stream pointer to next token
func (s *Stream) Next() *Stream {
	if s.current.next != nil {
		s.current = s.current.next
		if s.current.next == nil && s.p != nil { // lazy load and parse next data-chunk
			n := s.p.n
			s.p.parse()
			s.len += s.p.n - n
		}
		if s.historySize != 0 && s.current.id-s.head.id > s.historySize {
			t := s.head
			s.head = s.head.unlink()
			s.t.freeToken(t)
			s.len--
		}
	} else if s.current == undefToken {
		s.current = s.prev
		s.prev = nil
	} else {
		s.prev = s.current
		s.current = undefToken
	}
	return s
}

// Prev move pointer of stream to the next token.
func (s *Stream) Prev() *Stream {
	if s.current.prev != nil {
		s.current = s.current.prev
	} else if s.current == undefToken {
		s.current = s.next
		s.prev = nil
	} else {
		s.next = s.current
		s.current = undefToken
	}
	return s
}

// GoTo sets pointer of stream to the specific position.
func (s *Stream) GoTo(n int) *Stream {
	if n > s.current.id {
		for n != s.current.id && s.current != nil {
			s.Next()
		}
	} else if n < s.current.id {
		for s.current != nil && n != s.current.id {
			s.Prev()
		}
	}
	return s
}

// IsValid checks if stream is valid.
// This means that the pointer has not reached the end of the stream.
func (s *Stream) IsValid() bool {
	return s.current != undefToken
}

// HeadToken returns pointer to head-token
// Head-token may be changed if history size set.
func (s *Stream) HeadToken() *Token {
	return s.head
}

// CurrentToken always returns the token.
// If the pointer is not valid (see IsValid) CurrentToken will be returns TokenUndef token.
// Do not save result (Token) into variables — current token may be changed at any time.
func (s *Stream) CurrentToken() *Token {
	return s.current
}

// PrevToken returns previous token from the stream.
// If previous token doesn't exist method return TypeUndef token.
// Do not save result (Token) into variables — previous token may be changed at any time.
func (s *Stream) PrevToken() *Token {
	if s.current.prev != nil {
		return s.current.prev
	}
	return undefToken
}

// NextToken returns next token from the stream.
// If next token doesn't exist method return TypeUndef token.
// Do not save result (Token) into variables — next token may be changed at any time.
func (s *Stream) NextToken() *Token {
	if s.current.next != nil {
		return s.current.next
	}
	return undefToken
}

// GoNextIfNextIs move stream pointer to the next token if the next token has specific token keys.
// If keys matched pointer will be updated and method returned true. Otherwise, returned false.
func (s *Stream) GoNextIfNextIs(key int, otherKeys ...int) bool {
	if s.NextToken().Is(key, otherKeys...) {
		s.Next()
		return true
	}
	return false
}

// GetSnippet returns slice of tokens.
// Slice generated from current token position and include tokens before and after current token.
func (s *Stream) GetSnippet(before, after int) []Token {
	var segment []Token
	if s.current == undefToken {
		if s.prev != nil && before > s.prev.id-s.head.id {
			before = s.prev.id - s.head.id
		} else {
			before = 0
		}
	} else if before > s.current.id-s.head.id {
		before = s.current.id - s.head.id
	}
	if after > s.len-before-1 {
		after = s.len - before - 1
	}
	segment = make([]Token, before+after+1)
	var ptr *Token
	if s.next != nil {
		ptr = s.next
	} else if s.prev != nil {
		ptr = s.prev
	} else {
		ptr = s.current
	}
	for p := ptr; p != nil; p, before = ptr.prev, before-1 {
		segment[before] = Token{
			id:     ptr.id,
			key:    ptr.key,
			value:  ptr.value,
			line:   ptr.line,
			offset: ptr.offset,
			indent: ptr.indent,
			string: ptr.string,
		}
		if before <= 0 {
			break
		}
	}
	for p, i := ptr.next, 1; p != nil; p, i = p.next, i+1 {
		segment[before+i] = Token{
			id:     p.id,
			key:    p.key,
			value:  p.value,
			line:   p.line,
			offset: p.offset,
			indent: p.indent,
			string: p.string,
		}
		if i >= after {
			break
		}
	}
	return segment
}

// GetSnippetAsString returns tokens before and after current token as string.
// `maxStringLength` specify max length of each token string. Zero — unlimited token string length.
// If string greater than maxLength method removes some runes in the middle of the string.
func (s *Stream) GetSnippetAsString(before, after, maxStringLength int) string {
	segments := s.GetSnippet(before, after)
	str := make([]string, len(segments))
	for i, token := range segments {
		v := token.ValueString()
		if maxStringLength > 4 && len(v) > maxStringLength {
			str[i] = v[:maxStringLength/2] + "..." + v[maxStringLength/2+1:]
		} else {
			str[i] = v
		}
	}

	return strings.Join(str, "")
}