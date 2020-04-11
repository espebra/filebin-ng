package dbl

import (
	"database/sql"
	"errors"
	//"fmt"
	"github.com/dustin/go-humanize"
	"github.com/espebra/filebin2/ds"
	"path"
	"regexp"
	"strings"
	"time"
)

var invalidFilename = regexp.MustCompile("[^A-Za-z0-9-_=+,.]")

type FileDao struct {
	db *sql.DB
}

func (d *FileDao) ValidateInput(file *ds.File) error {
	// Replace all invalid characters with _
	file.Filename = invalidFilename.ReplaceAllString(file.Filename, "_")

	// . is not allowed as the first character
	if strings.HasPrefix(file.Filename, ".") {
		file.Filename = strings.Replace(file.Filename, ".", "_", 1)
	}

	// If the filename is empty, set it to _
	if len(file.Filename) == 0 {
		return errors.New("Filename not specified")
	}

	return nil
}

func (d *FileDao) GetById(id int) (file ds.File, found bool, err error) {
	sqlStatement := "SELECT id, bin_id, filename, deleted, mime, bytes, md5, sha256, downloads, updated, created FROM file WHERE id = $1 LIMIT 1"
	err = d.db.QueryRow(sqlStatement, id).Scan(&file.Id, &file.Bin, &file.Filename, &file.Deleted, &file.Mime, &file.Bytes, &file.MD5, &file.SHA256, &file.Downloads, &file.Updated, &file.Created)
	if err != nil {
		if err == sql.ErrNoRows {
			return file, false, nil
		} else {
			return file, false, err
		}
	}
	// https://github.com/lib/pq/issues/329
	file.Updated = file.Updated.UTC()
	file.Created = file.Created.UTC()
	file.UpdatedRelative = humanize.Time(file.Updated)
	file.CreatedRelative = humanize.Time(file.Created)
	file.BytesReadable = humanize.Bytes(file.Bytes)
	return file, true, nil
}

func (d *FileDao) GetByName(bin string, filename string) (file ds.File, found bool, err error) {
	sqlStatement := "SELECT id, bin_id, filename, deleted, mime, bytes, md5, sha256, downloads, updated, created FROM file WHERE bin_id = $1 AND filename = $2 LIMIT 1"
	err = d.db.QueryRow(sqlStatement, bin, filename).Scan(&file.Id, &file.Bin, &file.Filename, &file.Deleted, &file.Mime, &file.Bytes, &file.MD5, &file.SHA256, &file.Downloads, &file.Updated, &file.Created)
	if err != nil {
		if err == sql.ErrNoRows {
			return file, false, nil
		} else {
			return file, false, err
		}
	}
	// https://github.com/lib/pq/issues/329
	file.Updated = file.Updated.UTC()
	file.Created = file.Created.UTC()
	file.UpdatedRelative = humanize.Time(file.Updated)
	file.CreatedRelative = humanize.Time(file.Created)
	file.BytesReadable = humanize.Bytes(file.Bytes)
	return file, true, nil
}

func (d *FileDao) Upsert(file *ds.File) (err error) {
	if err := d.ValidateInput(file); err != nil {
		return err
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	sqlStatement := "SELECT id, bin_id, filename, deleted, mime, bytes, md5, sha256, downloads, updates, nonce, updated, created FROM file WHERE bin_id = $1 AND filename = $2 LIMIT 1"
	err = d.db.QueryRow(sqlStatement, file.Bin, file.Filename).Scan(&file.Id, &file.Bin, &file.Filename, &file.Deleted, &file.Mime, &file.Bytes, &file.MD5, &file.SHA256, &file.Downloads, &file.Updates, &file.Nonce, &file.Updated, &file.Created)
	if err != nil {
		if err == sql.ErrNoRows {
			deleted := 0
			downloads := 0
			updates := 0
			sqlStatement := "INSERT INTO file (bin_id, filename, deleted, mime, bytes, md5, sha256, downloads, updates, nonce, updated, created) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id"
			err := d.db.QueryRow(sqlStatement, file.Bin, file.Filename, deleted, file.Mime, file.Bytes, file.MD5, file.SHA256, downloads, updates, file.Nonce, now, now).Scan(&file.Id)
			if err != nil {
				return err
			}
			file.Downloads = uint64(downloads)
			file.Updates = uint64(updates)
			file.Deleted = deleted
			file.Updated = now
			file.Created = now
		} else {
			return err
		}
	}
	// https://github.com/lib/pq/issues/329
	file.Updated = file.Updated.UTC()
	file.Created = file.Created.UTC()

	file.UpdatedRelative = humanize.Time(file.Updated)
	file.CreatedRelative = humanize.Time(file.Created)
	file.BytesReadable = humanize.Bytes(file.Bytes)
	return nil
}

func (d *FileDao) Insert(file *ds.File) (err error) {
	if err := d.ValidateInput(file); err != nil {
		return err
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	deleted := 0
	downloads := 0
	updates := 0
	sqlStatement := "INSERT INTO file (bin_id, filename, deleted, mime, bytes, md5, sha256, downloads, updates, nonce, updated, created) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id"
	err = d.db.QueryRow(sqlStatement, file.Bin, file.Filename, file.Deleted, file.Mime, file.Bytes, file.MD5, file.SHA256, downloads, updates, file.Nonce, now, now).Scan(&file.Id)
	if err != nil {
		return err
	}
	file.Deleted = deleted
	file.Downloads = uint64(downloads)
	file.Updates = uint64(updates)
	file.Updated = now
	file.Created = now
	file.UpdatedRelative = humanize.Time(file.Updated)
	file.CreatedRelative = humanize.Time(file.Created)
	file.BytesReadable = humanize.Bytes(file.Bytes)
	return nil
}

func (d *FileDao) GetByBin(id string, deleted int) (files []ds.File, err error) {
	sqlStatement := "SELECT id, bin_id, filename, deleted, mime, bytes, md5, sha256, downloads, updates, nonce, updated, created FROM file WHERE bin_id = $1 AND deleted = $2 ORDER BY filename ASC"
	rows, err := d.db.Query(sqlStatement, id, deleted)
	if err != nil {
		return files, err
	}
	for rows.Next() {
		var file ds.File
		err = rows.Scan(&file.Id, &file.Bin, &file.Filename, &file.Deleted, &file.Mime, &file.Bytes, &file.MD5, &file.SHA256, &file.Downloads, &file.Updates, &file.Nonce, &file.Updated, &file.Created)
		if err != nil {
			return files, err
		}
		// https://github.com/lib/pq/issues/329
		file.Updated = file.Updated.UTC()
		file.Created = file.Created.UTC()
		file.UpdatedRelative = humanize.Time(file.Updated)
		file.CreatedRelative = humanize.Time(file.Created)
		file.BytesReadable = humanize.Bytes(file.Bytes)
		file.URL = path.Join(file.Bin, file.Filename)
		files = append(files, file)
	}
	return files, nil
}

func (d *FileDao) GetAll() (files []ds.File, err error) {
	sqlStatement := "SELECT id, bin_id, filename, deleted, mime, bytes, md5, sha256, downloads, updates, nonce, updated, created FROM file"
	rows, err := d.db.Query(sqlStatement)
	if err != nil {
		return files, err
	}
	for rows.Next() {
		var file ds.File
		err = rows.Scan(&file.Id, &file.Bin, &file.Filename, &file.Deleted, &file.Mime, &file.Bytes, &file.MD5, &file.SHA256, &file.Downloads, &file.Updates, &file.Nonce, &file.Updated, &file.Created)
		if err != nil {
			return files, err
		}
		// https://github.com/lib/pq/issues/329
		file.Updated = file.Updated.UTC()
		file.Created = file.Created.UTC()
		file.UpdatedRelative = humanize.Time(file.Updated)
		file.CreatedRelative = humanize.Time(file.Created)
		file.BytesReadable = humanize.Bytes(file.Bytes)
		files = append(files, file)
	}
	return files, nil
}

func (d *FileDao) Update(file *ds.File) (err error) {
	var id int
	now := time.Now().UTC().Truncate(time.Microsecond)
	sqlStatement := "UPDATE file SET filename = $1, deleted = $2, mime = $3, bytes = $4, md5 = $5, sha256 = $6, nonce = $7, updates = $8, updated = $9 WHERE id = $10 RETURNING id"
	err = d.db.QueryRow(sqlStatement, file.Filename, file.Deleted, file.Mime, file.Bytes, file.MD5, file.SHA256, file.Nonce, file.Updates, now, file.Id).Scan(&id)
	if err != nil {
		return err
	}
	file.Updated = now
	file.UpdatedRelative = humanize.Time(file.Updated)
	file.BytesReadable = humanize.Bytes(file.Bytes)
	return nil
}

func (d *FileDao) Delete(file *ds.File) (err error) {
	sqlStatement := "DELETE FROM file WHERE id = $1"
	res, err := d.db.Exec(sqlStatement, file.Id)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return errors.New("File does not exist")
	} else {
		return nil
	}
}

func (d *FileDao) RegisterDownload(file *ds.File) (err error) {
	sqlStatement := "UPDATE file SET downloads = downloads + 1 WHERE id = $1 RETURNING downloads"
	err = d.db.QueryRow(sqlStatement, file.Id).Scan(&file.Downloads)
	if err != nil {
		return err
	}
	return nil
}
