package parsers

// Node represents a node in the Abstract Syntax Tree.
// All AST nodes implement this interface for type safety and position tracking.
type Node interface {
	// GetStartPos returns the starting position of the node
	GetStartPos() Position
	// GetEndPos returns the ending position of the node
	GetEndPos() Position
	// NodeType returns the type of the node as a string
	NodeType() string
}

// BaseNode provides common functionality for all AST nodes.
type BaseNode struct {
	Position PositionRange
}

// GetStartPos implements the Node interface.
func (n *BaseNode) GetStartPos() Position {
	return n.Position.Start
}

// GetEndPos implements the Node interface.
func (n *BaseNode) GetEndPos() Position {
	return n.Position.End
}

// DocumentNode represents the root of an Internet Object document.
// It contains an optional header section and zero or more data sections.
type DocumentNode struct {
	BaseNode
	Header   *SectionNode   // Optional header section
	Sections []*SectionNode // Data sections
}

// NodeType returns the node type.
func (n *DocumentNode) NodeType() string {
	return "DocumentNode"
}

// SectionNode represents a section in the document.
// Sections can have optional names and schema references.
type SectionNode struct {
	BaseNode
	Child      Node   // The content of the section (ObjectNode or CollectionNode)
	NameToken  *Token // Optional section name token
	SchemaNode *Token // Optional schema reference token
}

// NodeType returns the node type.
func (n *SectionNode) NodeType() string {
	return "SectionNode"
}

// GetName returns the section name if present.
func (n *SectionNode) GetName() string {
	if n.NameToken != nil {
		if str, ok := n.NameToken.Value.(string); ok {
			return str
		}
	}
	// If no name token, try to get from schema node
	if n.SchemaNode != nil {
		if str, ok := n.SchemaNode.Value.(string); ok {
			// Remove $ prefix
			if len(str) > 0 && str[0] == '$' {
				return str[1:]
			}
			return str
		}
	}
	return "unnamed"
}

// GetSchemaName returns the schema name if present.
func (n *SectionNode) GetSchemaName() string {
	if n.SchemaNode != nil {
		if str, ok := n.SchemaNode.Value.(string); ok {
			return str
		}
	}
	return ""
}

// CollectionNode represents a collection of objects (using ~ delimiter).
type CollectionNode struct {
	BaseNode
	Children []Node // Array of ObjectNode or ErrorNode
}

// NodeType returns the node type.
func (n *CollectionNode) NodeType() string {
	return "CollectionNode"
}

// ObjectNode represents an object with key-value pairs or indexed values.
// It can be enclosed in {} or be an open object.
type ObjectNode struct {
	BaseNode
	Members      []*MemberNode // Array of members
	OpenBracket  *Token        // Optional opening bracket token
	CloseBracket *Token        // Optional closing bracket token
	IsOpen       bool          // True if this is an open object (no braces)
}

// NodeType returns the node type.
func (n *ObjectNode) NodeType() string {
	return "ObjectNode"
}

// ArrayNode represents an array of values enclosed in [].
type ArrayNode struct {
	BaseNode
	Elements     []Node // Array elements
	OpenBracket  *Token // Opening bracket token
	CloseBracket *Token // Closing bracket token
}

// NodeType returns the node type.
func (n *ArrayNode) NodeType() string {
	return "ArrayNode"
}

// MemberNode represents a key-value pair in an object.
// If Key is nil, it's an indexed value (array-like member).
type MemberNode struct {
	BaseNode
	Key   *Token // Optional key token
	Value Node   // Value (can be any node type)
}

// NodeType returns the node type.
func (n *MemberNode) NodeType() string {
	return "MemberNode"
}

// HasKey returns true if this member has a key.
func (n *MemberNode) HasKey() bool {
	return n.Key != nil
}

// TokenNode is a leaf node wrapping a single token.
// It represents primitive values (strings, numbers, booleans, etc.).
type TokenNode struct {
	BaseNode
	Token *Token
}

// NodeType returns the node type.
func (n *TokenNode) NodeType() string {
	return "TokenNode"
}

// GetValue returns the token's value.
func (n *TokenNode) GetValue() interface{} {
	if n.Token != nil {
		return n.Token.Value
	}
	return nil
}

// ErrorNode represents a parsing error embedded in the AST.
// This allows the parser to continue after errors and collect multiple errors.
type ErrorNode struct {
	BaseNode
	Error error // The error that occurred
}

// NodeType returns the node type.
func (n *ErrorNode) NodeType() string {
	return "ErrorNode"
}

// NewDocumentNode creates a new document node.
func NewDocumentNode(header *SectionNode, sections []*SectionNode) *DocumentNode {
	var start, end Position

	if header != nil {
		start = header.GetStartPos()
	} else if len(sections) > 0 {
		start = sections[0].GetStartPos()
	}

	if len(sections) > 0 {
		end = sections[len(sections)-1].GetEndPos()
	} else if header != nil {
		end = header.GetEndPos()
	}

	return &DocumentNode{
		BaseNode: BaseNode{Position: NewPositionRange(start, end)},
		Header:   header,
		Sections: sections,
	}
}

// NewSectionNode creates a new section node.
func NewSectionNode(child Node, nameToken, schemaNode *Token) *SectionNode {
	var start, end Position

	if nameToken != nil {
		start = nameToken.GetStartPos()
	} else if schemaNode != nil {
		start = schemaNode.GetStartPos()
	} else if child != nil {
		start = child.GetStartPos()
	}

	if child != nil {
		end = child.GetEndPos()
	} else if schemaNode != nil {
		end = schemaNode.GetEndPos()
	} else if nameToken != nil {
		end = nameToken.GetEndPos()
	}

	return &SectionNode{
		BaseNode:   BaseNode{Position: NewPositionRange(start, end)},
		Child:      child,
		NameToken:  nameToken,
		SchemaNode: schemaNode,
	}
}

// NewCollectionNode creates a new collection node.
func NewCollectionNode(children []Node) *CollectionNode {
	var start, end Position
	if len(children) > 0 {
		start = children[0].GetStartPos()
		end = children[len(children)-1].GetEndPos()
	}

	return &CollectionNode{
		BaseNode: BaseNode{Position: NewPositionRange(start, end)},
		Children: children,
	}
}

// NewObjectNode creates a new object node.
func NewObjectNode(members []*MemberNode, openBracket, closeBracket *Token) *ObjectNode {
	var start, end Position
	isOpen := openBracket == nil

	if openBracket != nil {
		start = openBracket.GetStartPos()
	} else if len(members) > 0 {
		start = members[0].GetStartPos()
	}

	if closeBracket != nil {
		end = closeBracket.GetEndPos()
	} else if len(members) > 0 {
		end = members[len(members)-1].GetEndPos()
	}

	return &ObjectNode{
		BaseNode:     BaseNode{Position: NewPositionRange(start, end)},
		Members:      members,
		OpenBracket:  openBracket,
		CloseBracket: closeBracket,
		IsOpen:       isOpen,
	}
}

// NewArrayNode creates a new array node.
func NewArrayNode(elements []Node, openBracket, closeBracket *Token) *ArrayNode {
	start := openBracket.GetStartPos()
	end := closeBracket.GetEndPos()

	return &ArrayNode{
		BaseNode:     BaseNode{Position: NewPositionRange(start, end)},
		Elements:     elements,
		OpenBracket:  openBracket,
		CloseBracket: closeBracket,
	}
}

// NewMemberNode creates a new member node.
func NewMemberNode(value Node, key *Token) *MemberNode {
	var start, end Position

	if key != nil {
		start = key.GetStartPos()
	} else {
		start = value.GetStartPos()
	}

	end = value.GetEndPos()

	return &MemberNode{
		BaseNode: BaseNode{Position: NewPositionRange(start, end)},
		Key:      key,
		Value:    value,
	}
}

// NewTokenNode creates a new token node.
func NewTokenNode(token *Token) *TokenNode {
	return &TokenNode{
		BaseNode: BaseNode{Position: token.Position},
		Token:    token,
	}
}

// NewErrorNode creates a new error node.
func NewErrorNode(err error, pos PositionRange) *ErrorNode {
	return &ErrorNode{
		BaseNode: BaseNode{Position: pos},
		Error:    err,
	}
}
