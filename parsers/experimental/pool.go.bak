package parsers

import (
	"sync"
)

// Object pools for reducing allocations
var (
	// Token pool
	tokenPool = sync.Pool{
		New: func() interface{} {
			return &Token{}
		},
	}

	// Position pool
	positionPool = sync.Pool{
		New: func() interface{} {
			return &Position{}
		},
	}

	// PositionRange pool
	positionRangePool = sync.Pool{
		New: func() interface{} {
			return &PositionRange{}
		},
	}

	// Node pools
	objectNodePool = sync.Pool{
		New: func() interface{} {
			return &ObjectNode{}
		},
	}

	arrayNodePool = sync.Pool{
		New: func() interface{} {
			return &ArrayNode{}
		},
	}

	memberNodePool = sync.Pool{
		New: func() interface{} {
			return &MemberNode{}
		},
	}

	sectionNodePool = sync.Pool{
		New: func() interface{} {
			return &SectionNode{}
		},
	}

	collectionNodePool = sync.Pool{
		New: func() interface{} {
			return &CollectionNode{}
		},
	}

	// Slice pools for reducing slice allocations
	tokenSlicePool = sync.Pool{
		New: func() interface{} {
			s := make([]*Token, 0, 64)
			return &s
		},
	}

	memberSlicePool = sync.Pool{
		New: func() interface{} {
			s := make([]*MemberNode, 0, 16)
			return &s
		},
	}

	nodeSlicePool = sync.Pool{
		New: func() interface{} {
			s := make([]Node, 0, 16)
			return &s
		},
	}
)

// GetToken retrieves a token from the pool
func GetToken() *Token {
	return tokenPool.Get().(*Token)
}

// PutToken returns a token to the pool
func PutToken(t *Token) {
	if t != nil {
		// Reset the token
		t.Type = ""
		t.SubType = ""
		t.Value = nil
		t.Raw = ""
		tokenPool.Put(t)
	}
}

// GetPosition retrieves a position from the pool
func GetPosition() *Position {
	return positionPool.Get().(*Position)
}

// PutPosition returns a position to the pool
func PutPosition(p *Position) {
	if p != nil {
		positionPool.Put(p)
	}
}

// GetPositionRange retrieves a position range from the pool
func GetPositionRange() *PositionRange {
	return positionRangePool.Get().(*PositionRange)
}

// PutPositionRange returns a position range to the pool
func PutPositionRange(pr *PositionRange) {
	if pr != nil {
		positionRangePool.Put(pr)
	}
}

// GetObjectNode retrieves an object node from the pool
func GetObjectNode() *ObjectNode {
	return objectNodePool.Get().(*ObjectNode)
}

// PutObjectNode returns an object node to the pool
func PutObjectNode(n *ObjectNode) {
	if n != nil {
		n.Members = nil
		objectNodePool.Put(n)
	}
}

// GetArrayNode retrieves an array node from the pool
func GetArrayNode() *ArrayNode {
	return arrayNodePool.Get().(*ArrayNode)
}

// PutArrayNode returns an array node to the pool
func PutArrayNode(n *ArrayNode) {
	if n != nil {
		n.Elements = nil
		arrayNodePool.Put(n)
	}
}

// GetMemberNode retrieves a member node from the pool
func GetMemberNode() *MemberNode {
	return memberNodePool.Get().(*MemberNode)
}

// PutMemberNode returns a member node to the pool
func PutMemberNode(n *MemberNode) {
	if n != nil {
		n.Value = nil
		n.Key = nil
		memberNodePool.Put(n)
	}
}

// GetTokenSlice retrieves a token slice from the pool
func GetTokenSlice() *[]*Token {
	slice := tokenSlicePool.Get().(*[]*Token)
	*slice = (*slice)[:0] // Reset length to 0
	return slice
}

// PutTokenSlice returns a token slice to the pool
func PutTokenSlice(s *[]*Token) {
	if s != nil && cap(*s) <= 1024 { // Don't pool very large slices
		tokenSlicePool.Put(s)
	}
}

// GetMemberSlice retrieves a member slice from the pool
func GetMemberSlice() *[]*MemberNode {
	slice := memberSlicePool.Get().(*[]*MemberNode)
	*slice = (*slice)[:0]
	return slice
}

// PutMemberSlice returns a member slice to the pool
func PutMemberSlice(s *[]*MemberNode) {
	if s != nil && cap(*s) <= 256 {
		memberSlicePool.Put(s)
	}
}

// GetNodeSlice retrieves a node slice from the pool
func GetNodeSlice() *[]Node {
	slice := nodeSlicePool.Get().(*[]Node)
	*slice = (*slice)[:0]
	return slice
}

// PutNodeSlice returns a node slice to the pool
func PutNodeSlice(s *[]Node) {
	if s != nil && cap(*s) <= 256 {
		nodeSlicePool.Put(s)
	}
}
