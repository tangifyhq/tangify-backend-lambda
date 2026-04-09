package main

import (
	"time"

	"github.com/google/uuid"
)

type CommonUtils struct {
}

var commonUtils *CommonUtils

func NewCommonUtils() *CommonUtils {
	if commonUtils == nil {
		commonUtils = &CommonUtils{}
		return commonUtils
	}
	return commonUtils
}

func (c *CommonUtils) GetCurrentTimestamp() int64 {
	return time.Now().UnixMilli()
}

func (c *CommonUtils) GenerateUniqueID(prefix *string) string {
	if prefix == nil || *prefix == "" {
		return uuid.New().String()
	}
	return *prefix + "_" + uuid.New().String()
}
