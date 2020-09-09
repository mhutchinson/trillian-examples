module github.com/google/trillian-examples/sumdbaudit

go 1.14

require (
	github.com/cenkalti/backoff/v4 v4.0.2
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/google/trillian/merkle/compact v0.0.0
	github.com/mattn/go-sqlite3 v1.14.2
	golang.org/x/mod v0.3.0
	golang.org/x/sync v0.0.0-20200625203802-6e8e738ad208
)

replace github.com/google/trillian/merkle/compact v0.0.0 => github.com/mhutchinson/trillian/merkle/compact v0.0.0-20200909095250-ef5133df1450
