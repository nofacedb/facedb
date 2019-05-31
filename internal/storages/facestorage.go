package storages

import (
	"database/sql"
	"time"

	"github.com/kshvakov/clickhouse"
	"github.com/nofacedb/facedb/internal/proto"
	"github.com/pkg/errors"
)

// FaceStorage ...
type FaceStorage struct {
	db           *sql.DB
	sineBoundary float64
}

// CreateFaceStorage ...
func CreateFaceStorage(db *sql.DB, sineBoundary float64) *FaceStorage {
	return &FaceStorage{
		db:           db,
		sineBoundary: sineBoundary,
	}
}

// InsertControlObjectsQuery ...
const InsertControlObjectsQuery = `
INSERT INTO
    control_objects
    (id, ts, passport,
     surname, name, patronymic,
     sex, birthdate,
     phone_num, email, address)
VALUES
    (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
`

// InsertControlObjects ...
func (fs *FaceStorage) InsertControlObjects(cobs []proto.ControlObject) error {
	tx, err := fs.db.Begin()
	if err != nil {
		return errors.Wrap(err, "unable to begin bulk insert")
	}
	stmt, err := tx.Prepare(InsertControlObjectsQuery)
	if err != nil {
		return errors.Wrap(err, "unable to prepare SQL-statement")
	}
	defer stmt.Close()

	for i, cob := range cobs {
		if _, err := stmt.Exec(
			clickhouse.UUID(cob.ID),
			cob.TS,
			cob.Passport,
			cob.Surname,
			cob.Name,
			cob.Patronymic,
			cob.Sex,
			cob.BirthDate,
			cob.PhoneNum,
			cob.Email,
			cob.Address,
		); err != nil {
			return errors.Wrapf(err, "unable to execute %d-th part of bulk insert", i+1)
		}
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "unable to commit bulk insert")
	}

	return nil
}

// SelectControlObjectByPassportQuery ...
const SelectControlObjectByPassportQuery = `
SELECT
    id, ts, passport,
    surname, name, patronymic,
    sex, birthdate,
    phone_num, email, address
FROM
    control_objects FINAL
WHERE
    (passport = ?);
`

// SelectControlObjectByPassport ...
func (fs *FaceStorage) SelectControlObjectByPassport(passport string) (*proto.ControlObject, error) {
	rows, err := fs.db.Query(SelectControlObjectByPassportQuery, passport)
	if err != nil {
		return nil, errors.Wrap(err, "unable to execute query")
	}
	defer rows.Close()

	if rows.Next() {
		cob := proto.CreateDefaultControlObject()
		if err := rows.Scan(
			&(cob.ID), &(cob.TS), &(cob.Passport),
			&(cob.Surname), &(cob.Name), &(cob.Patronymic),
			&(cob.Sex), &(cob.BirthDate),
			&(cob.PhoneNum), &(cob.Email), &(cob.Address)); err != nil {
			return nil, errors.Wrap(err, "unable to unmarshal query result")
		}
		return cob, nil
	}

	return proto.CreateDefaultControlObject(), nil
}

// InsertFFVsQuery ...
const InsertFFVsQuery = `
INSERT INTO
    facial_features
    (id, cob_id, img_id, fb, ff)
VALUES
    (?, ?, ?, ?, ?);
`

// FFV ...
type FFV struct {
	ID                   string
	CobID                string
	ImgID                string
	FaceBox              proto.FaceBox
	FacialFeaturesVector proto.FacialFeaturesVector
}

// InsertFFVs ...
func (fs *FaceStorage) InsertFFVs(ffvs []FFV) error {
	tx, err := fs.db.Begin()
	if err != nil {
		return errors.Wrap(err, "unable to begin bulk write transaction")
	}
	stmt, err := tx.Prepare(InsertFFVsQuery)
	if err != nil {
		return errors.Wrap(err, "unable to prepare InsertFFQuery statemet")
	}
	defer stmt.Close()
	for _, ffv := range ffvs {
		if _, err := stmt.Exec(
			clickhouse.UUID(ffv.ID),
			clickhouse.UUID(ffv.CobID),
			clickhouse.UUID(ffv.ImgID),
			clickhouse.Array(ffv.FaceBox),
			clickhouse.Array(ffv.FacialFeaturesVector),
		); err != nil {
			return errors.Wrap(err, "unable to execute part of bulk write transaction. Rollbacking")
		}
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "unable to commit bulk write transaction. Rollbacking")
	}

	return nil
}

// InsertImgsQuery ...
const InsertImgsQuery = `
INSERT INTO imgs
    (id, ts, path, face_ids)
VALUES
    (?, ?, ?, ?);`

// Img ...
type Img struct {
	ID      string
	TS      time.Time
	Path    string
	FaceIDs []string
}

// InsertImgs ...
func (fs *FaceStorage) InsertImgs(imgs []Img) error {
	tx, err := fs.db.Begin()
	if err != nil {
		return errors.Wrap(err, "unable to begin bulk write transaction")
	}
	stmt, err := tx.Prepare(InsertImgsQuery)
	if err != nil {
		return errors.Wrap(err, "unable to prepare InsertImgQuery statemet")
	}
	defer stmt.Close()

	for _, img := range imgs {
		if _, err := stmt.Exec(
			clickhouse.UUID(img.ID),
			img.TS,
			img.Path,
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

// SelectImgsByControlObjectQuery ...
const SelectImgsByControlObjectQuery = `
SELECT
    *
FROM
    imgs
WHERE
    has(face_ids, ?);
`

// SelectImgsByControlObject ...
func (fs *FaceStorage) SelectImgsByControlObject(cob *proto.ControlObject) ([]Img, error) {
	rows, err := fs.db.Query(SelectImgsByControlObjectQuery, cob.ID)
	if err != nil {
		return nil, errors.Wrap(err, "unable to execute query")
	}
	defer rows.Close()

	imgs := make([]Img, 0, 128)
	i := 0
	for rows.Next() {
		imgs = append(imgs, Img{})
		if err := rows.Scan(
			&(imgs[i].ID), &(imgs[i].TS),
			&(imgs[i].Path), &(imgs[i].FaceIDs),
		); err != nil {
			return nil, errors.Wrap(err, "unable to unmarshal query result")
		}
		i++
	}

	return imgs, nil
}

/*
To find the most suitable object of control, we can use the sine:
the more similar the vectors, the closer the sine of the angle between them to zero.
*/

// SelectControlObjectByFFVQuery ...
const SelectControlObjectByFFVQuery = `
SELECT
     cob_id, ts, passport,
     surname, name, patronymic,
     sex, birthdate,
     phone_num, email, address
FROM
(
    SELECT
        control_objects.id AS cob_id,
        control_objects.ts AS ts,
        control_objects.passport AS passport,
        control_objects.surname AS surname,
        control_objects.name AS name,
        control_objects.patronymic AS patronymic,
        control_objects.sex AS sex,
        control_objects.birthdate AS birthdate,
        control_objects.phone_num AS phone_num,
        control_objects.email AS email,
        control_objects.address AS address
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

// SelectControlObjectByFFV ...
func (fs *FaceStorage) SelectControlObjectByFFV(ff []float64) (*proto.ControlObject, error) {
	rows, err := fs.db.Query(SelectControlObjectByFFVQuery,
		clickhouse.Array(ff),
		clickhouse.Array(ff),
		fs.sineBoundary,
	)
	if err != nil {
		return nil, errors.Wrap(err, "unable to execute query")
	}
	defer rows.Close()

	if rows.Next() {
		cob := proto.CreateDefaultControlObject()
		if err := rows.Scan(
			&(cob.ID), &(cob.TS), &(cob.Passport),
			&(cob.Surname), &(cob.Name), &(cob.Patronymic),
			&(cob.Sex), &(cob.BirthDate),
			&(cob.PhoneNum), &(cob.Email), &(cob.Address),
		); err != nil {
			return nil, errors.Wrap(err, "unable to unmarshal query result")
		}
		return cob, nil
	}

	return proto.CreateDefaultControlObject(), nil
}
