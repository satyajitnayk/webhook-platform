### create test database

```shell
createdb webhooks_test
psql webhooks_test < migrations/001_init.sql
```

### Run test cases

```shell
# all tests
go test ./...
# specific test
go test -run TestClaimDeliveryOnlyOneWorkerWins -v

# check for race condition
go test -race ./...
```
