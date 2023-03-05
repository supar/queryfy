### Queryfy

Queryfy is a play service to try produce concurrency patterns and represents http server, which obtains POST request with the URLs list in JSON format and asquire all these, at the end returns reponse to the clent in JSON format.

- Incoming requests limit: 100 (default).
- On client cancelation drops outgoing requests.
- In case any URL fail returns an error message.
- URL limit 20
- Max outgoing requests 20
- server graceful shutdown

Http Responses
- 200 - ok
- 400 - bad request data
- 408 - any outgoing request timeout
- 422 - any outgoing request fails

#### Run

Help

```
go run . -h
```

Default (listen :8080)

```
go run .
```

Test OK
```
curl -i -X POST -d '["https://en.wikipedia.org/wiki/Sun"]' http://localhost:8080/
```

Test remote FAIL

```
curl -i -X POST -d '["https://google.com/q=help"]' http://localhost:8080/
```