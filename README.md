# Distributed-Mutual-Exclusion
BSDISYS1KU-20222, mandatory handin 4

##  How to run

 1. Run `main.go` in 3 separate terminals

    ```
    $ go run main.go 0
    $ go run main.go 1
    $ go run main.go 2
    ```

It is hardcoded for 3 peers with port 5000, 5001 and 5002. 


3. A peer will create a log.txt in the root directory under the name `peer(*)-log.txt`, with `*` being the bound port.
