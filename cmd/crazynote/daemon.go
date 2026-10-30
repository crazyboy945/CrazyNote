package main

// stripDaemonFlag 从参数列表中去掉 -daemon / --daemon 标志，
// 避免派生的子进程再次尝试脱离（造成重复 fork）。
//
// 其余参数（包括 -log、-data、-appdata、-addr）全部保留。
func stripDaemonFlag(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if a == "-daemon" || a == "--daemon" {
			continue
		}
		out = append(out, a)
	}
	return out
}
