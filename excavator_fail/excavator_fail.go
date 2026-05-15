package fail

fail

/*
This is a non-compiling file that has been added to explicitly ensure that CI fails.
It also contains the command that caused the failure and its output.
Remove this file if debugging locally.

./godelw verify failed after updating godel plugins and assets

Command that caused error:
./godelw exec -- go fix ./. ./cmd ./dister ./dister/bin ./dister/bin/config ./dister/bin/config/internal/v0 ./dister/bin/integration_test ./dister/disterfactory ./dister/distertaskprovider ./dister/distertaskprovider/distertaskproviderapi ./dister/distertester ./dister/manual ./dister/manual/config ./dister/manual/config/internal/legacy ./dister/manual/config/internal/v0 ./dister/manual/integration_test ./dister/osarchbin ./dister/osarchbin/config ./dister/osarchbin/config/internal/legacy ./dister/osarchbin/config/internal/v0 ./dister/osarchbin/integration_test ./distgo ./distgo/artifacts ./distgo/build ./distgo/build/imports ./distgo/clean ./distgo/config ./distgo/config/internal/legacy ./distgo/config/internal/v0 ./distgo/dist ./distgo/docker ./distgo/printproducts ./distgo/projectversion ./distgo/publish ./distgo/run ./distgo/testfuncs ./distgotaskprovider ./dockerbuilder ./dockerbuilder/defaultdockerbuilder ./dockerbuilder/defaultdockerbuilder/config ./dockerbuilder/defaultdockerbuilder/config/internal/v0 ./dockerbuilder/defaultdockerbuilder/integration_test ./dockerbuilder/dockerbuilderfactory ./dockerbuilder/dockerbuildertester ./integration_test ./internal/assetapi ./internal/assetapi/distertaskproviderinternal ./internal/assetapi/distgotaskproviderinternal ./internal/cmdinternal ./internal/files ./projectversioner ./projectversioner/git ./projectversioner/git/config ./projectversioner/git/config/internal/v0 ./projectversioner/git/integration_test ./projectversioner/projectversionerfactory ./projectversioner/projectversiontester ./projectversioner/script ./projectversioner/script/config ./projectversioner/script/config/internal/v0 ./projectversioner/script/integration_test ./publisher ./publisher/artifactory ./publisher/artifactory/config ./publisher/artifactory/config/internal/v0 ./publisher/artifactory/integration_test ./publisher/github ./publisher/github/config ./publisher/github/config/internal/v0 ./publisher/github/integration_test ./publisher/maven ./publisher/mavenlocal ./publisher/mavenlocal/config ./publisher/mavenlocal/config/internal/v0 ./publisher/mavenlocal/integration_test ./publisher/publisherfactory ./publisher/publishertester

Output:
stat /repo/pkg/git/cmd: directory not found
stat /repo/pkg/git/dister: directory not found
stat /repo/pkg/git/dister/bin: directory not found
stat /repo/pkg/git/dister/bin/config: directory not found
stat /repo/pkg/git/dister/bin/config/internal/v0: directory not found
stat /repo/pkg/git/dister/bin/integration_test: directory not found
stat /repo/pkg/git/dister/disterfactory: directory not found
stat /repo/pkg/git/dister/distertaskprovider: directory not found
stat /repo/pkg/git/dister/distertaskprovider/distertaskproviderapi: directory not found
stat /repo/pkg/git/dister/distertester: directory not found
stat /repo/pkg/git/dister/manual: directory not found
stat /repo/pkg/git/dister/manual/config: directory not found
stat /repo/pkg/git/dister/manual/config/internal/legacy: directory not found
stat /repo/pkg/git/dister/manual/config/internal/v0: directory not found
stat /repo/pkg/git/dister/manual/integration_test: directory not found
stat /repo/pkg/git/dister/osarchbin: directory not found
stat /repo/pkg/git/dister/osarchbin/config: directory not found
stat /repo/pkg/git/dister/osarchbin/config/internal/legacy: directory not found
stat /repo/pkg/git/dister/osarchbin/config/internal/v0: directory not found
stat /repo/pkg/git/dister/osarchbin/integration_test: directory not found
stat /repo/pkg/git/distgo: directory not found
stat /repo/pkg/git/distgo/artifacts: directory not found
stat /repo/pkg/git/distgo/build: directory not found
stat /repo/pkg/git/distgo/build/imports: directory not found
stat /repo/pkg/git/distgo/clean: directory not found
stat /repo/pkg/git/distgo/config: directory not found
stat /repo/pkg/git/distgo/config/internal/legacy: directory not found
stat /repo/pkg/git/distgo/config/internal/v0: directory not found
stat /repo/pkg/git/distgo/dist: directory not found
stat /repo/pkg/git/distgo/docker: directory not found
stat /repo/pkg/git/distgo/printproducts: directory not found
stat /repo/pkg/git/distgo/projectversion: directory not found
stat /repo/pkg/git/distgo/publish: directory not found
stat /repo/pkg/git/distgo/run: directory not found
stat /repo/pkg/git/distgo/testfuncs: directory not found
stat /repo/pkg/git/distgotaskprovider: directory not found
stat /repo/pkg/git/dockerbuilder: directory not found
stat /repo/pkg/git/dockerbuilder/defaultdockerbuilder: directory not found
stat /repo/pkg/git/dockerbuilder/defaultdockerbuilder/config: directory not found
stat /repo/pkg/git/dockerbuilder/defaultdockerbuilder/config/internal/v0: directory not found
stat /repo/pkg/git/dockerbuilder/defaultdockerbuilder/integration_test: directory not found
stat /repo/pkg/git/dockerbuilder/dockerbuilderfactory: directory not found
stat /repo/pkg/git/dockerbuilder/dockerbuildertester: directory not found
stat /repo/pkg/git/integration_test: directory not found
stat /repo/pkg/git/internal/assetapi: directory not found
stat /repo/pkg/git/internal/assetapi/distertaskproviderinternal: directory not found
stat /repo/pkg/git/internal/assetapi/distgotaskproviderinternal: directory not found
stat /repo/pkg/git/internal/cmdinternal: directory not found
stat /repo/pkg/git/internal/files: directory not found
stat /repo/pkg/git/projectversioner: directory not found
stat /repo/pkg/git/projectversioner/git: directory not found
stat /repo/pkg/git/projectversioner/git/config: directory not found
stat /repo/pkg/git/projectversioner/git/config/internal/v0: directory not found
stat /repo/pkg/git/projectversioner/git/integration_test: directory not found
stat /repo/pkg/git/projectversioner/projectversionerfactory: directory not found
stat /repo/pkg/git/projectversioner/projectversiontester: directory not found
stat /repo/pkg/git/projectversioner/script: directory not found
stat /repo/pkg/git/projectversioner/script/config: directory not found
stat /repo/pkg/git/projectversioner/script/config/internal/v0: directory not found
stat /repo/pkg/git/projectversioner/script/integration_test: directory not found
stat /repo/pkg/git/publisher: directory not found
stat /repo/pkg/git/publisher/artifactory: directory not found
stat /repo/pkg/git/publisher/artifactory/config: directory not found
stat /repo/pkg/git/publisher/artifactory/config/internal/v0: directory not found
stat /repo/pkg/git/publisher/artifactory/integration_test: directory not found
stat /repo/pkg/git/publisher/github: directory not found
stat /repo/pkg/git/publisher/github/config: directory not found
stat /repo/pkg/git/publisher/github/config/internal/v0: directory not found
stat /repo/pkg/git/publisher/github/integration_test: directory not found
stat /repo/pkg/git/publisher/maven: directory not found
stat /repo/pkg/git/publisher/mavenlocal: directory not found
stat /repo/pkg/git/publisher/mavenlocal/config: directory not found
stat /repo/pkg/git/publisher/mavenlocal/config/internal/v0: directory not found
stat /repo/pkg/git/publisher/mavenlocal/integration_test: directory not found
stat /repo/pkg/git/publisher/publisherfactory: directory not found
stat /repo/pkg/git/publisher/publishertester: directory not found
Error: exit status 1

*/
