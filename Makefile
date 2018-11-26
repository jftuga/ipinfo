
PROG = ipinfo

$(PROG) : $(PROG).go
	VERS='$(shell git describe --abbrev=0)' ; \
	go build -ldflags "-s -w -X main.BuildTime=$$VERS" $(PROG).go

clean:
	rm -f $(PROG) *~ .??*~
