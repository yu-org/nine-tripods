
testall:
	go test -v ./...

test_mevless:
	go test -v ./MEVless/tests/single_node_test.go

test_poa:
	go test -v ./consensus/poa/tests/single_node_test.go

reset:
	@rm -rf */yu