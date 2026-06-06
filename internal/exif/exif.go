package exif

import (
	"io"
	"strings"
	"time"

	goexif "github.com/rwcarlsen/goexif/exif"
)

type GPS struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type Result struct {
	Found    bool
	TakenAt  *time.Time
	GPS      *GPS
	Metadata map[string]any
}

func Extract(reader io.Reader) (Result, error) {
	result := Result{Metadata: map[string]any{"exif_available": false}}
	x, err := goexif.Decode(reader)
	if err != nil {
		result.Metadata["exif_error"] = err.Error()
		return result, nil
	}
	result.Found = true
	result.Metadata["exif_available"] = true
	if raw := stringTag(x, goexif.DateTimeOriginal); raw != "" {
		result.Metadata["exif_datetime_original_raw"] = raw
	} else if raw := stringTag(x, goexif.DateTime); raw != "" {
		result.Metadata["exif_datetime_raw"] = raw
	}
	if tz, _ := x.TimeZone(); tz != nil {
		if dt, err := x.DateTime(); err == nil {
			taken := dt.UTC()
			result.TakenAt = &taken
			result.Metadata["exif_datetime_timezone"] = tz.String()
		}
	} else {
		result.Metadata["exif_datetime_policy"] = "timezone-less EXIF datetime stored as raw metadata only"
	}
	if lat, lon, err := x.LatLong(); err == nil {
		result.GPS = &GPS{Lat: lat, Lon: lon}
		result.Metadata["gps_lat"] = lat
		result.Metadata["gps_lon"] = lon
	}
	copyStringTag(result.Metadata, x, goexif.Make, "camera_make")
	copyStringTag(result.Metadata, x, goexif.Model, "camera_model")
	copyStringTag(result.Metadata, x, goexif.LensMake, "lens_make")
	copyStringTag(result.Metadata, x, goexif.LensModel, "lens_model")
	copyIntTag(result.Metadata, x, goexif.Orientation, "orientation")
	copyStringifiedTag(result.Metadata, x, goexif.FocalLength, "focal_length")
	copyStringifiedTag(result.Metadata, x, goexif.ExposureTime, "exposure_time")
	copyStringifiedTag(result.Metadata, x, goexif.FNumber, "f_number")
	copyStringifiedTag(result.Metadata, x, goexif.ISOSpeedRatings, "iso_speed")
	return result, nil
}

func copyStringTag(metadata map[string]any, x *goexif.Exif, field goexif.FieldName, key string) {
	if value := stringTag(x, field); value != "" {
		metadata[key] = value
	}
}

func stringTag(x *goexif.Exif, field goexif.FieldName) string {
	tag, err := x.Get(field)
	if err != nil {
		return ""
	}
	value, err := tag.StringVal()
	if err != nil {
		return ""
	}
	value = strings.TrimSpace(strings.TrimRight(value, "\x00"))
	return value
}

func copyIntTag(metadata map[string]any, x *goexif.Exif, field goexif.FieldName, key string) {
	tag, err := x.Get(field)
	if err != nil {
		return
	}
	value, err := tag.Int(0)
	if err != nil {
		return
	}
	metadata[key] = value
}

func copyStringifiedTag(metadata map[string]any, x *goexif.Exif, field goexif.FieldName, key string) {
	tag, err := x.Get(field)
	if err != nil {
		return
	}
	metadata[key] = strings.Trim(tag.String(), "\"")
}
