-- facedb is a basic database for all facerecognition data.
CREATE DATABASE IF NOT EXISTS `facedb`;

-- control_objects is a table for people, we are interested in (control objects).
CREATE TABLE IF NOT EXISTS `facedb`.`control_objects`
(
    `id`         UUID     DEFAULT generateUUIDv4(), -- surrogate key.
    `db_ts`      DateTime DEFAULT now(),            -- internal  (database) timestamp.
    `ts`         DateTime DEFAULT now(),            -- external  timestamp.
    `passport`   String,                            -- natural   key.
    `surname`    String   DEFAULT '-',
    `name`       String   DEFAULT '-',
    `patronymic` String   DEFAULT '-',
    `sex`        Enum8('male' = 0, 'female' = 1, '-' = 2) DEFAULT '-',
    `birthdate`  String   DEFAULT '-',
    `phone_num`  String   DEFAULT '-',
    `email`      String   DEFAULT '-',
    `address`    String   DEFAULT '-'
) ENGINE = ReplacingMergeTree(`db_ts`)
  ORDER BY `id`;

-- facial_features is a table for all control objects facial features vectors.
CREATE TABLE IF NOT EXISTS `facedb`.`facial_features`
(
    `id`         UUID DEFAULT generateUUIDv4(), -- surrogate key.
    `cob_id`     UUID,                          -- facedb.control_objects FK.
    `img_id`     UUID,                          -- facedb.imgs FK.
    `fb`         Array(UInt64),                 -- facebox.
    `ff`         Array(Float64)                 -- facial features.
) ENGINE = MergeTree()
  ORDER BY `id`
  PARTITION BY (`cob_id`, `img_id`);

-- embedded_facial_features is a view for average control objects facial features vectors.
-- they are counted as the arithmetic mean of all facial features vectors.
CREATE MATERIALIZED VIEW IF NOT EXISTS  `facedb`.`embedded_facial_features`
ENGINE = AggregatingMergeTree() ORDER BY `cob_id`
AS SELECT
   `cob_id`,
   avgForEach(`ff`) AS `eff`,
   toInt8(arraySum(`eff`) /
    (sqrt(arraySum(arrayMap(x -> x * x, `eff`))) *
    sqrt(128.0)) * 10.0) AS `cosine_on_ort`
FROM `facedb`.`facial_features`
GROUP BY `cob_id`
ORDER BY `cosine_on_ort` ASC, `cob_id` ASC;

-- imgs is a table for all saved images.
CREATE TABLE IF NOT EXISTS `facedb`.`imgs`
(
    `id`       UUID        DEFAULT generateUUIDv4(), -- surrogate key.
    `ts`       DateTime    DEFAULT now(),
    `path`     String,
    `face_ids` Array(UUID)                           -- facedb.facial_features FKs.
) ENGINE = MergeTree()
  ORDER BY `id`;
