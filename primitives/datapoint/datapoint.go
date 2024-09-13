package datapoint

type BinaryClassifyDatapoint struct {
	Instruction string
	Text        string
	Examples    []*BinaryClassifyDatapoint
	Response    *bool
}

type ClassifyDatapoint struct {
	Instruction string
	Text        string
	Options     []string
	Examples    []*ClassifyDatapoint
	Response    *int
}

type ParseForceDatapoint struct {
	Instruction string
	Text        string
	V           any
	Examples    []*ParseForceDatapoint
	Response    *any
}

type ScoreDatapoint struct {
	Instruction string
	Text        string
	Min         int
	Max         int
	Examples    []*ScoreDatapoint
	Response    *int
}

type GenerateDatapoint struct {
	Instruction string
	Text        string
	Examples    []*GenerateDatapoint
	Response    *string
}
