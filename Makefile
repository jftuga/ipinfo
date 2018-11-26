
PROG = ipinfo

$(PROG) : $(PROG).go
	go build -ldflags="-s -w" $(PROG).go

clean:
	rm $(PROG)
