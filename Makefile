
PROG = ipinfo

$(PROG) : $(PROG).go
	VERS='$(shell awk -F/ '/refs\/tags\// {print $$NF}' .git/packed-refs | tail -1)' ; \
	go build -ldflags "-s -w -X main.BuildTime=$$VERS" $(PROG).go

clean:
	rm -f $(PROG) *~ .??*~
