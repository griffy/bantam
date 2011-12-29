include $(GOROOT)/src/Make.inc

TARG=github.com/griffy/bantam
GOFMT=gofmt -s -spaces=true -tabindent=false -tabwidth=4

GOFILES=\
  bantam.go\

include $(GOROOT)/src/Make.pkg

format:
	${GOFMT} -w ${GOFILES}

