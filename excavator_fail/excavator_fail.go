package fail

fail

/*
This is a non-compiling file that has been added to explicitly ensure that CI fails.
It also contains the command that caused the failure and its output.
Remove this file if debugging locally.

go mod operation failed. This may mean that there are legitimate dependency issues with the "go.mod" definition in the repository and the updates performed by the gomod check. This branch can be cloned locally to debug the issue.

Command that caused error:
./godelw exec -- go get golang.org/x/sys

Output:
go: module golang.org/x/sys: Get "https://proxy.golang.org/golang.org/x/sys/@v/list": dial tcp 142.251.211.209:443: i/o timeout
Error: exit status 1

*/
