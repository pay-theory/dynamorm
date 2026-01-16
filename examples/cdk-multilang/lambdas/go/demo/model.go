package demo

import "os"

type DemoItem struct {
	PK     string `dynamorm:"pk,attr:PK" json:"PK"`
	SK     string `dynamorm:"sk,attr:SK" json:"SK"`
	Value  string `dynamorm:"attr:value,omitempty" json:"value,omitempty"`
	Lang   string `dynamorm:"attr:lang,omitempty" json:"lang,omitempty"`
	Secret string `dynamorm:"encrypted,attr:secret,omitempty" json:"secret,omitempty"`
}

func (DemoItem) TableName() string {
	return os.Getenv("TABLE_NAME")
}
