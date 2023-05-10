# folder name of the package of interest and supporting library
PKGNAME = naming storage common

GSONFILE = gson-2.8.6.jar

# where are all the source files for main package and test code
SRCFILES = $(foreach pkg,$(PKGNAME),$(pkg)/*.java)
TESTFILES = test/*.java test/util/*.java $(foreach pkg,$(PKGNAME),test/$(pkg)/*.java)

# javadoc output directory and library url
DOCDIR = doc
DOCLINK = https://docs.oracle.com/en/java/javase/19/docs/api

.PHONY: build test checkpoint clean docs docs-test
.SILENT: build test checkpoint clean docs docs-test

# compile all source files
build:
	javac -cp $(GSONFILE) $(TESTFILES) common/*.java
	go build storage/StorageServer.go
	# TODO (if needed): add command to compile your naming and storage server

# run tests
test: build
	java -cp .:$(GSONFILE) test.Lab3FinalTests

checkpoint: build
	java -cp .:$(GSONFILE) test.Lab3CheckpointTests
    
# delete all class files and docs, leaving only source
clean:
	rm -rf $(SRCFILES:.java=.class) $(TESTFILES:.java=.class) $(DOCDIR) $(DOCDIR)-test StorageServer storage0_root .DS_Store
	
# generate documentation for the package of interest
docs:
	javadoc -cp .:$(GSONFILE) -private -link $(DOCLINK) -d $(DOCDIR) common
	
# generate documentation for the test suite
docs-test:
	javadoc -cp .:$(GSONFILE) -private -link $(DOCLINK) -d $(DOCDIR)-test test test.util $(foreach pkg,$(PKGNAME),test.$(pkg))
