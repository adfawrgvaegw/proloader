package define

type Size struct {
	Width, Height, Length int
}

func (s *Size) GetWidth() int {
	return s.Width
}

func (s *Size) GetHeight() int {
	return s.Height
}

func (s *Size) GetLength() int {
	return s.Length
}

func (s *Size) GetVolume() int {
	return s.Width * s.Height * s.Length
}

func (s *Size) GetChunkXCount() int {
	return (s.Width + 16 - 1) / 16
}

func (s *Size) GetChunkZCount() int {
	return (s.Length + 16 - 1) / 16
}

func (s *Size) GetChunkCount() int {
	return s.GetChunkXCount() * s.GetChunkZCount()
}
