package facedb

import (
	"database/sql"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/kshvakov/clickhouse"
	"github.com/nofacedb/facedb/internal/cfgparser"
	"github.com/pkg/errors"
)

// FaceStorage ...
type FaceStorage struct {
	db           *sql.DB
	SineBoundary float64
	logger       *log.Logger
}

const dSNPattern = "tcp://%s?username=%s&password=%s&database=%s&read_timeout=%d&write_timeout=%d&debug=%v"

// CreateFaceStorage ...
func CreateFaceStorage(cfg *cfgparser.FaceStorageCFG, logger *log.Logger) (*FaceStorage, error) {
	dSN := fmt.Sprintf(dSNPattern, cfg.Addr,
		cfg.User, cfg.Passwd, cfg.DefaultDB,
		cfg.ReadTimeoutMS/1000, cfg.WriteTimeoutMS/1000, cfg.Debug)
	db, err := sql.Open("clickhouse", dSN)
	if err != nil {
		return nil, errors.Wrap(err, "unable connect to facedb")
	}
	pingTimes := 0
	for pingTimes = 0; pingTimes < cfg.NPing; pingTimes++ {
		err := db.Ping()
		if err == nil {
			break
		}

		if exception, ok := err.(*clickhouse.Exception); ok {
			logger.Errorf("[%d] %s \n%s\n", exception.Code, exception.Message, exception.StackTrace)
		} else {
			logger.Error(errors.Wrapf(err, "unable to ping ClickHouse DB for %d time", pingTimes+1))
		}
	}
	if pingTimes == cfg.NPing {
		return nil, fmt.Errorf("unable to ping ClickHouse DB for %d times", cfg.NPing)
	}

	return &FaceStorage{
		db:           db,
		SineBoundary: cfg.SineBoundary,
		logger:       logger,
	}, nil
}

// UNKNOWNFIELD ...
const UNKNOWNFIELD = "-"

const (
	// MALESEX ...
	MALESEX = `male`
	// FEMALESEX ...
	FEMALESEX = `female`
	// UNKNOWNSEX ...
	UNKNOWNSEX = `unknown`
)

// InsertCOBQuery ...
const InsertCOBQuery = `
INSERT INTO
    control_objects
    (ts, name, patronymic, surname, passport, sex, phone_num)
VALUES
    (?, ?, ?, ?, ?, ?, ?);
`

// COB ...
type COB struct {
	ID         *string `json:"id"`
	dbTS       *time.Time
	TS         *time.Time
	Name       string `json:"name"`
	Patronymic string `json:"patronymic"`
	Surname    string `json:"surname"`
	Passport   string `json:"passport"`
	Sex        string `json:"sex"`
	PhoneNum   string `json:"phone_num"`
}

// CreateUnknownCOB ...
func CreateUnknownCOB() COB {
	ID := UNKNOWNFIELD
	return COB{
		ID:         &ID,
		Name:       UNKNOWNFIELD,
		Patronymic: UNKNOWNFIELD,
		Surname:    UNKNOWNFIELD,
		Passport:   UNKNOWNFIELD,
		Sex:        UNKNOWNSEX,
		PhoneNum:   UNKNOWNFIELD,
	}
}

// CmpCOBsByID ...
func CmpCOBsByID(cob1, cob2 COB) bool {
	return *(cob1.ID) == *(cob2.ID)
}

// CmpCOBsByAll ...
func CmpCOBsByAll(cob1, cob2 COB) bool {
	return (*(cob1.ID) == *(cob2.ID)) &&
		(cob1.Name == cob2.Name) &&
		(cob1.Patronymic == cob2.Patronymic) &&
		(cob1.Surname == cob2.Surname) &&
		(cob1.Passport == cob2.Passport) &&
		(cob1.Sex == cob2.Sex) &&
		(cob1.PhoneNum == cob2.PhoneNum)
}

// InsertCOB ...
func (fs *FaceStorage) InsertCOB(cobs []COB) error {
	for _, cob := range cobs {
		if cob.TS == nil {
			return fmt.Errorf("TimeStamp can't be nil")
		}
	}
	tx, err := fs.db.Begin()
	if err != nil {
		return errors.Wrap(err, "unable to begin bulk write transaction")
	}
	stmt, err := tx.Prepare(InsertCOBQuery)
	if err != nil {
		return errors.Wrap(err, "unable to prepare InsertCOBQuery statemet")
	}
	defer stmt.Close()

	for _, cob := range cobs {
		if _, err := stmt.Exec(
			*cob.TS,
			cob.Name,
			cob.Patronymic,
			cob.Surname,
			cob.Passport,
			cob.Sex,
			cob.PhoneNum,
		); err != nil {
			return errors.Wrap(err, "unable to execute part of bulk write transaction. Rollbacking")
		}
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "unable to commit bulk write transaction. Rollbacking")
	}

	return nil
}

// SelectCOBByPassportQuery ...
const SelectCOBByPassportQuery = `
SELECT
    control_objects.id, control_objects.ts, control_objects.name,
    control_objects.patronymic, control_objects.surname, control_objects.passport,
    control_objects.sex, control_objects.phone_num
FROM
    control_objects
WHERE
    (passport = ?);
`

// SelectCOBByPassport ...
func (fs *FaceStorage) SelectCOBByPassport(passport string) (COB, error) {
	rows, err := fs.db.Query(SelectCOBByPassportQuery,
		passport,
	)
	if err != nil {
		return COB{}, errors.Wrap(err, "unable to execute query")
	}
	defer rows.Close()

	for rows.Next() {
		cob := COB{
			ID: new(string),
			TS: new(time.Time),
		}
		if err := rows.Scan(cob.ID, cob.TS,
			&(cob.Name), &(cob.Patronymic), &(cob.Surname),
			&(cob.Passport), &(cob.Sex), &(cob.PhoneNum)); err != nil {
			return COB{}, errors.Wrap(err, "unable to unmarshal query result")
		}
		return cob, nil
	}

	return CreateUnknownCOB(), nil
}

// InsertFFQuery ...
const InsertFFQuery = `
INSERT INTO
    facial_features
    (cob_id, img_id, fb, ff)
VALUES
    (?, ?, ?, ?);
`

// FF ...
type FF struct {
	ID    *string
	COBID string
	IMGID string
	Box   []uint64
	FF    []float64
}

// InsertFF ...
func (fs *FaceStorage) InsertFF(ffs []FF) error {
	tx, err := fs.db.Begin()
	if err != nil {
		return errors.Wrap(err, "unable to begin bulk write transaction")
	}
	stmt, err := tx.Prepare(InsertFFQuery)
	if err != nil {
		return errors.Wrap(err, "unable to prepare InsertCOBQuery statemet")
	}
	defer stmt.Close()
	for _, ff := range ffs {
		if _, err := stmt.Exec(
			clickhouse.UUID(ff.COBID),
			clickhouse.UUID(ff.IMGID),
			clickhouse.Array(ff.Box),
			clickhouse.Array(ff.FF),
		); err != nil {
			return errors.Wrap(err, "unable to execute part of bulk write transaction. Rollbacking")
		}
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "unable to commit bulk write transaction. Rollbacking")
	}

	return nil
}

// InsertIMGQuery ...
const InsertIMGQuery = `
INSERT INTO imgs
    (ts, faces)
VALUES
    (?, ?);`

// IMG ...
type IMG struct {
	ID      *string
	TS      time.Time
	FaceIDs []string
}

// InsertIMG ...
func (fs *FaceStorage) InsertIMG(imgs []IMG) error {
	tx, err := fs.db.Begin()
	if err != nil {
		return errors.Wrap(err, "unable to begin bulk write transaction")
	}
	stmt, err := tx.Prepare(InsertIMGQuery)
	if err != nil {
		return errors.Wrap(err, "unable to prepare InsertCOBQuery statemet")
	}
	defer stmt.Close()

	for _, img := range imgs {
		if _, err := stmt.Exec(
			img.TS,
			clickhouse.Array(img.FaceIDs),
		); err != nil {
			return errors.Wrap(err, "unable to execute part of bulk write transaction. Rollbacking")
		}
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "unable to commit bulk write transaction. Rollbacking")
	}

	return nil
}

/*
To find the most suitable object of control, we can use the sine:
the more similar the vectors, the closer the sine of the angle between them to zero.
*/

// SelectCOBByFFQuery ...
const SelectCOBByFFQuery = `
SELECT
    cob_id, ts,
    name, patronymic, surname,
    passport, sex, phone_num
FROM
(
    SELECT
        control_objects.id AS cob_id,
        control_objects.ts AS ts,
        control_objects.name AS name,
        control_objects.patronymic AS patronymic,
        control_objects.surname AS surname,
        control_objects.passport AS passport,
        control_objects.sex AS sex,
        control_objects.phone_num AS phone_num
    FROM
       control_objects
) JOIN
(
    SELECT
        cob_id, avgForEach(ff) AS eff
    FROM
        facial_features
    GROUP BY cob_id
) USING cob_id
WHERE
    ((1 - pow(arraySum(arrayMap((x, y) -> (x * y), eff, array(?))), 2) / 
          (arraySum(arrayMap(x -> x * x, array(?))) *
           arraySum(arrayMap(x -> x * x, eff)))) < ?)
    LIMIT 1
`

// SelectCOBByFF ...
func (fs *FaceStorage) SelectCOBByFF(ff []float64) (COB, error) {
	rows, err := fs.db.Query(SelectCOBByFFQuery,
		clickhouse.Array(ff),
		clickhouse.Array(ff),
		fs.SineBoundary,
	)
	if err != nil {
		return COB{}, errors.Wrap(err, "unable to execute query")
	}
	defer rows.Close()

	for rows.Next() {
		cob := COB{
			ID: new(string),
			TS: new(time.Time),
		}
		if err := rows.Scan(cob.ID, cob.TS,
			&(cob.Name), &(cob.Patronymic), &(cob.Surname),
			&(cob.Passport), &(cob.Sex), &(cob.PhoneNum)); err != nil {
			return COB{}, errors.Wrap(err, "unable to unmarshal query result")
		}
		return cob, nil
	}

	return CreateUnknownCOB(), nil
}
