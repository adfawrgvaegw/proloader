package pymarshal

func absInt32(n int32) int32 {
	if n < 0 {
		return -n
	} else {
		return n
	}
}
