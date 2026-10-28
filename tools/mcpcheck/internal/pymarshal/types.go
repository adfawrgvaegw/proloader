package pymarshal

const (
	TYPE_NULL           = '0' // 0x30
	TYPE_NONE           = 'N' // 0x4e
	TYPE_FALSE          = 'F' // 0x46
	TYPE_TRUE           = 'T' // 0x54
	TYPE_STOPITER       = 'S' // 0x53
	TYPE_ELLIPSIS       = '.' // 0x2e
	TYPE_INT            = 'i' // 0x69
	TYPE_INT64          = 'I' // 0x49
	TYPE_FLOAT          = 'f' // 0x66
	TYPE_BINARY_FLOAT   = 'g' // 0x67
	TYPE_COMPLEX        = 'x' // 0x78
	TYPE_BINARY_COMPLEX = 'y' // 0x79
	TYPE_LONG           = 'l' // 0x6c
	TYPE_STRING         = 's' // 0x73
	TYPE_INTERNED       = 't' // 0x74
	TYPE_STRINGREF      = 'R' // 0x52
	TYPE_TUPLE          = '(' // 0x28
	TYPE_LIST           = '[' // 0x5b
	TYPE_DICT           = '{' // 0x7b
	TYPE_CODE           = 'c' // 0x63
	TYPE_UNICODE        = 'u' // 0x75
	TYPE_UNKNOWN        = '?' // 0x3f
	TYPE_SET            = '<' // 0x3c
	TYPE_FROZENSET      = '>' // 0x3e
)
