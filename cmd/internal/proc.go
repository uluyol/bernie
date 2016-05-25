package internal

type ProcReq struct {
	Name string
	Cmd  []string
	Env  []string
	PWD  string

	NoStart bool
}
