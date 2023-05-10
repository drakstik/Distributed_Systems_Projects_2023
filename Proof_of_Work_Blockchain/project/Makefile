# folder name of the package of interest

.PHONY: test 
.SILENT: test

# compile the remote library.
build:
	cd project; go build main.go

# run conformance tests.
test: build
	cd project; go test 