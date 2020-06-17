package parsers

import "regexp"

// ReWS expression checks whether specified character is a white space or not
var /* const */ ReWS = regexp.MustCompile(`[\s]`)

// ReSep checks whether the specified character is an IO separtor or not
var /* const */ ReSep = regexp.MustCompile(`[\{\}\[\]\:\,~]`)

// ReNotSep ensures that the specified character is not an IO separator
var /* const */ ReNotSep = regexp.MustCompile(`[^\{\}\[\]\:\,~]`)

// ReNotRegularString ensure that the specified character is not a regular string encloser
var /* const */ ReNotRegularString = regexp.MustCompile(`[^\"]`)

// ReRegularString ensures that the specified string is an IO regular string
var /* const */ ReRegularString = regexp.MustCompile(`^\"(?:[^\"\\]|\\.)*\"$`)

// ReRawString ensures that the specified string is an IO raw string
var /* const */ ReRawString = regexp.MustCompile(`^'((?:''|[^'])*)'$`)

// ReNumber ensures that the spefied string is an IO number
var /* const */ ReNumber = regexp.MustCompile(`^([-+]?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)$`)
