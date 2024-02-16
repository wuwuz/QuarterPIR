# Efficient Pre-processing PIR Without Public-Key Cryptography (QuaterPIR)
This is a prototype implementation of the QuaterPIR, a private information retrieval(PIR) algorithm that allows a client to access a database without the server knowing the querying index.
The algorithm details can be found in the paper (https://eprint.iacr.org/2023/1574).
QuaterPIR is the optimized single-server PIR scheme described in the Appendix B, which has $O_\lambda(n^{1/4})$ online communication and $O(\sqrt{n})$ computation per query.

**Note**: This repo also includes the original Piano PIR (IEEE S&P 24) implementation (https://eprint.iacr.org/2023/452).

**Warning**: The code is not audited and not for any serious commercial or real-world use case. Please use it only for educational purpose.

### Prerequisite:
1. Install Go(https://go.dev/doc/install). We use the version of Go 1.19.
2. For developing, please install gRPC(https://grpc.io/docs/languages/go/quickstart/)

### Running Experiments:
0. First run `go mod tidy` to download the dependency.
1. In one terminal, `go run server/server.go -port 50051`. This sets up the server. The server will store the whole DB in the RAM, so please ensure there's enough memory.
2. In another terminal, `go run ec24/ec24_client.go -ip localhost:50051 -thread 8`. This runs the PIR experiment with one setup phase for a window of $\sqrt{n}\ln(n)$-queries and follows with the online phase of up to 1000 queries. The ip flag denotes the server's address. The thread denotes how many threads are used in the setup phase. The 1GB experiment typically runs for a few minutes. The 64GB and the 100GB experiments run for no more than 2 hours on an AWS m5.8xlarge instance. (Tips: if you just want to see the performance, you can run it with flag `--ignoreOffline`. This will skip the long preprocessing phase and run the online phase assuming all the hints are zero.)
3. View the results in `output.txt`.
4. You can run the Piano PIR scheme to see the comparison: `go run client_new/client_new.go -ip localhost:50051 -thread 8`. The results will still be in `output.txt`.

#### Different DB configuration:
1. The first two integers in `config.txt` denote `N` and `DBSeed`. `N` denotes the number of entries in the database. `DBSeed` denotes the random seed to generate the DB. The client will use the seed only for verifying the correctness. The code only reads the integers in the first line. You can change it according to your need.
2. In `util/util.go`, you can change the `DBEntrySize` constant to change the entry size, e.g. 8bytes, 32bytes, 256bytes.

### Developing
1. The server implementation is in `server/server.go`.
2. The client implementation is in `ec24/ec24_client.go`.
3. Common utilities are in `util/util.go`, including the PRF and the `DBEntry` definition.
4. The messages exchanged by servers and client are defined in `query/query.proto`. If you change it, run `bash proto.sh` to generate the corresponding server and client API. You should implement those API later.

### Contact
Mingxun Zhou(mingxunz@andrew.cmu.edu)