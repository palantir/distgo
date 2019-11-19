module github.com/palantir/distgo

go 1.13

require (
	github.com/google/go-github/v28 v28.1.1
	github.com/jtacoma/uritemplates v1.0.0
	github.com/mholt/archiver/v3 v3.3.0
	github.com/nmiyake/pkg/dirs v1.0.0
	github.com/nmiyake/pkg/errorstringer v1.0.0 // indirect
	github.com/nmiyake/pkg/gofiles v1.0.0
	github.com/palantir/godel/v2 v2.22.0
	github.com/palantir/pkg v0.0.0-20190110173045-b2b34217111b
	github.com/pkg/errors v0.8.1
	github.com/spf13/cobra v0.0.2
	github.com/spf13/pflag v1.0.0
	github.com/stretchr/testify v1.4.0
	github.com/termie/go-shutil v0.0.0-20140729215957-bcacb06fecae
	golang.org/x/oauth2 v0.0.0-20180821212333-d2e6202438be
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e // indirect
	golang.org/x/tools v0.0.0-20191109212701-97ad0ed33101
	gopkg.in/cheggaaa/pb.v1 v1.0.22
	gopkg.in/yaml.v2 v2.2.2
)

replace github.com/nmiyake/pkg => github.com/nmiyake/pkg v0.0.0
