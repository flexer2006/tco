package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
)

const clusterUntitledSlug = "cluster-untitled"

var cyrillicTransliteration = map[rune]string{
	'а': "a",
	'б': "b",
	'в': "v",
	'г': "g",
	'д': "d",
	'е': "e",
	'ё': "yo",
	'ж': "zh",
	'з': "z",
	'и': "i",
	'й': "y",
	'к': "k",
	'л': "l",
	'м': "m",
	'н': "n",
	'о': "o",
	'п': "p",
	'р': "r",
	'с': "s",
	'т': "t",
	'у': "u",
	'ф': "f",
	'х': "h",
	'ц': "ts",
	'ч': "ch",
	'ш': "sh",
	'щ': "shch",
	'ъ': "",
	'ы': "y",
	'ь': "",
	'э': "e",
	'ю': "yu",
	'я': "ya",
}

type cluster struct {
	centroid             Vector
	noteIDs              []NoteID
	createdAt, updatedAt time.Time
	id, name, slug       string
}

func newCluster(id, name string, slug string, centroid Vector, noteIDs []NoteID, createdAt, updatedAt time.Time) (cluster, error) {
	if strings.TrimSpace(id) == "" {
		return cluster{}, errors.New("id must not be empty")
	}
	if strings.TrimSpace(name) == "" {
		return cluster{}, errors.New("name must not be empty")
	}
	if centroid.Dimension() == 0 {
		return cluster{}, errors.New("centroid must not be empty")
	}
	if len(noteIDs) == 0 {
		return cluster{}, errors.New("note_ids must not be empty")
	}
	for i, noteID := range noteIDs {
		if strings.TrimSpace(noteID.String()) == "" {
			return cluster{}, fmt.Errorf("note_ids[%d]: must not be empty", i)
		}
	}
	if err := validateClusterUTC("created_at", createdAt); err != nil {
		return cluster{}, err
	}
	if err := validateClusterUTC("updated_at", updatedAt); err != nil {
		return cluster{}, err
	}
	canonicalSlug := Slugify(name)
	if slug != "" && slug != canonicalSlug {
		return cluster{}, fmt.Errorf("slug: expected %q for %q, got %q", canonicalSlug, name, slug)
	}
	return cluster{
		id:        id,
		name:      name,
		slug:      canonicalSlug,
		centroid:  centroid,
		noteIDs:   slices.Clone(noteIDs),
		createdAt: createdAt.UTC(),
		updatedAt: updatedAt.UTC(),
	}, nil
}

func (c cluster) idValue() string       { return c.id }
func (c cluster) nameValue() string     { return c.name }
func (c cluster) slugValue() string     { return c.slug }
func (c cluster) centroidValue() Vector { return c.centroid }

func validateClusterUTC(field string, value time.Time) error {
	if value.IsZero() {
		return fmt.Errorf("%s must not be zero", field)
	}
	if value.Location() != time.UTC {
		return fmt.Errorf("%s must be UTC", field)
	}
	return nil
}

func Slugify(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return clusterUntitledSlug
	}
	var b strings.Builder
	b.Grow(len(trimmed))
	lastWasHyphen := true
	for _, r := range trimmed {
		switch {
		case isCyrillic(r):
			transliterated := cyrillicTransliteration[unicode.ToLower(r)]
			if transliterated == "" {
				continue
			}
			b.WriteString(transliterated)
			lastWasHyphen = false
		case isASCIIAlphaNumeric(r):
			b.WriteRune(unicode.ToLower(r))
			lastWasHyphen = false
		default:
			if !lastWasHyphen && b.Len() > 0 {
				b.WriteByte('-')
				lastWasHyphen = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return clusterUntitledSlug
	}
	return slug
}

func isASCIIAlphaNumeric(r rune) bool {
	return r <= unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r))
}

func isCyrillic(r rune) bool {
	return r >= 'а' && r <= 'я' || r == 'ё' || r == 'Ё' || r >= 'А' && r <= 'Я'
}
