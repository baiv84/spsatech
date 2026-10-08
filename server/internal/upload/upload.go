// Package upload разбирает multipart-формы с фото.
package upload

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"unicode/utf8"

	"spsatech/helpdesk/internal/files"
	"spsatech/helpdesk/internal/store"
)

const (
	MaxPhotos       = 5
	MaxPhotoBytes   = 10 << 20
	maxRequestBytes = MaxPhotos*MaxPhotoBytes + 1<<20
	maxFieldBytes   = 32 << 10
	PhotoField      = "photos"
)

// Error — ошибка, которую можно показать пользователю.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

type Form struct {
	Fields map[string]string
	Photos []store.NewAttachment
}

// Discard удаляет уже сохранённые фото — если заявку или сообщение сохранить не удалось.
func (f *Form) Discard(fs *files.Storage) {
	for _, p := range f.Photos {
		fs.Remove(p.StorageKey)
	}
}

// Parse потоково читает форму: фото сразу пишутся на диск.
// При ошибке уже сохранённые файлы удаляются. Ошибка типа *Error — для пользователя,
// любая другая — внутренняя.
func Parse(w http.ResponseWriter, r *http.Request, fs *files.Storage) (*Form, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, &Error{http.StatusBadRequest, "bad_request", "Ожидается multipart/form-data"}
	}
	form := &Form{Fields: map[string]string{}}
	fail := func(err error) (*Form, error) {
		form.Discard(fs)
		return nil, err
	}

	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fail(classify(err))
		}

		name := part.FormName()
		if part.FileName() == "" {
			val, err := io.ReadAll(io.LimitReader(part, maxFieldBytes+1))
			if err != nil {
				return fail(classify(err))
			}
			if len(val) > maxFieldBytes {
				return fail(&Error{http.StatusBadRequest, "bad_request", "Слишком длинное поле " + name})
			}
			form.Fields[name] = string(val)
			continue
		}
		if name != PhotoField {
			return fail(&Error{http.StatusBadRequest, "bad_request", "Неизвестное поле " + name})
		}
		// Пустой файл приходит из браузера, если фото не выбрали.
		saved, err := savePhoto(fs, part)
		if errors.Is(err, errEmpty) {
			continue
		}
		if err != nil {
			return fail(classify(err))
		}
		form.Photos = append(form.Photos, saved)
		if len(form.Photos) > MaxPhotos {
			return fail(&Error{http.StatusBadRequest, "validation",
				fmt.Sprintf("Можно приложить не больше %d фото", MaxPhotos)})
		}
	}
	return form, nil
}

var (
	errTooLarge = errors.New("photo too large")
	errEmpty    = errors.New("empty file")
)

func classify(err error) error {
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		return &Error{http.StatusRequestEntityTooLarge, "too_large", "Слишком большой объём файлов"}
	case errors.Is(err, errTooLarge):
		return &Error{http.StatusRequestEntityTooLarge, "too_large", "Фото больше 10 МБ"}
	case errors.Is(err, files.ErrUnsupportedType):
		return &Error{http.StatusBadRequest, "validation", "Можно прикладывать только фото (JPEG, PNG, WebP)"}
	case errors.Is(err, multipart.ErrMessageTooLarge), errors.Is(err, io.ErrUnexpectedEOF):
		return &Error{http.StatusBadRequest, "bad_request", "Некорректный запрос"}
	}
	return err
}

func savePhoto(fs *files.Storage, part *multipart.Part) (store.NewAttachment, error) {
	saved, err := fs.Save(io.LimitReader(part, MaxPhotoBytes+1))
	if errors.Is(err, files.ErrEmpty) {
		return store.NewAttachment{}, errEmpty
	}
	if err != nil {
		return store.NewAttachment{}, err
	}
	if saved.Size > MaxPhotoBytes {
		fs.Remove(saved.Key)
		return store.NewAttachment{}, errTooLarge
	}
	fileName := filepath.Base(part.FileName())
	if utf8.RuneCountInString(fileName) > 200 || !utf8.ValidString(fileName) {
		fileName = "photo" + files.AllowedTypes[saved.ContentType]
	}
	return store.NewAttachment{
		FileName: fileName, ContentType: saved.ContentType, SizeBytes: saved.Size, StorageKey: saved.Key,
	}, nil
}
