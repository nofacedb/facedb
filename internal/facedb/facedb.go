package facedb

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/kshvakov/clickhouse"
	"github.com/nofacedb/facedb/internal/cfgparser"
	"github.com/pkg/errors"
)

// FaceStorage ...
type FaceStorage struct {
	db           *sql.DB
	SineBoundary float64
}

const dSNPattern = "tcp://%s?username=%s&password=%s&database=%s&read_timeout=%d&write_timeout=%d&debug=%v"

// CreateFaceStorage ...
func CreateFaceStorage(cfg cfgparser.FaceStorageCFG) (*FaceStorage, error) {
	dSN := fmt.Sprintf(dSNPattern, cfg.Addr,
		cfg.User, cfg.Passwd, cfg.DefaultDB,
		cfg.ReadTimeoutS, cfg.WriteTimeoutS, cfg.Debug)
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
			fmt.Printf("[%d] %s \n%s\n", exception.Code, exception.Message, exception.StackTrace)
		} else {
			fmt.Println(err)
		}
	}
	if pingTimes == cfg.NPing {
		return nil, fmt.Errorf("unable ping database for %d times", cfg.NPing)
	}

	return &FaceStorage{
		db: db,
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
INSERT INTO control_objects
    (ts, name, patronymic, surname, passport, sex, phone_num)
VALUES
    (?, ?, ?, ?, ?, ?, ?);`

// COB ...
type COB struct {
	ID         *string
	dbTS       *time.Time
	TS         *time.Time
	Name       string
	Patronymic string
	Surname    string
	Passport   string
	Sex        string
	PhoneNum   string
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

// InsertFFQuery ...
const InsertFFQuery = `
INSERT INTO facial_features
    (cob_id, img_id, fb, ff)
VALUES
    (?, ?, ?, ?);`

// FF ...
type FF struct {
	ID    *string
	COBID string
	IMGID string
	FB    []uint64
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
			ff.COBID,
			ff.IMGID,
			clickhouse.Array(ff.FB),
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
SELECT control_objects.id, control_objects.ts, control_objects.name,
       control_objects.patronymic, control_objects.surname, control_objects.passport,
       control_objects.sex, control_objects.phone_num
    FROM control_objects JOIN embedded_facial_features
    ON control_objects.id = embedded_facial_features.cob_id
    WHERE ((1 -
            pow(arraySum(arrayMap((x, y) -> (x * y), array(embedded_facial_features.eff), array(?))), 2) /
            (arraySum(arrayMap(x -> x * x, array(?))) *
             arraySum(arrayMap(x -> x * x, array(embedded_facial_features.eff))))) < ?)
    LIMIT 1
`

// SelectCOBByFF ...
func (fs *FaceStorage) SelectCOBByFF(ff []float64) (*COB, error) {
	rows, err := fs.db.Query(SelectCOBByFFQuery,
		clickhouse.Array(ff),
		clickhouse.Array(ff),
		fs.SineBoundary,
	)
	if err != nil {
		return nil, errors.Wrap(err, "unable to execute query")
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
			return nil, errors.Wrap(err, "unable to unmarshal query result")
		}
		return &cob, nil
	}

	return nil, nil
}
