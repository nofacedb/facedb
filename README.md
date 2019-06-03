# facedb

**nofacedb/facedb** is DB 'connector' for **NoFaceDB** program complex.

## Tech

**facedb** uses a number of open source projects to work properly:
- [ClickHouse](https://clickhouse.yandex/) - fast OLAP system from Russia;
- [Logrus](https://github.com/sirupsen/logrus) - easy-extensible logger;

## Installation

**faceprocessor** requires [Python](https://www.python.org/) v3.6+ to run.

Get **faceprocessor** (and other microservices), install the dependencies from requirements.txt, and now You are ready to find faces!

```sh
$ git clone https://github.com/nofacedb/facedb
$ cd facedb
$ go build main.go
```
## HowTo
**facedb** is a scheduler for all image processing tasks: processing images, pushing them to DB, adding new control objects, etc.

## Many thanks to:

- Igor Vishnyakov and Mikhail Pinchukov - my scientific directors;
