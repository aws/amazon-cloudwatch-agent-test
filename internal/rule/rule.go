// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package rule

import (
	"log"
)

type Rule[T any] struct {
	Conditions []ICondition[T]
}

func (r *Rule[T]) Evaluate(target T) (bool, error) {
	for _, c := range r.Conditions {
		log.Printf("Evaluating condition: %v", c.GetName())
		success, err := c.Evaluate(target)
		if err != nil {
			log.Printf("Error was %v", err)
			return false, err
		}
		log.Printf("Evaluate result: %v", success)
		if !success {
			return false, nil
		}
	}
	return true, nil
}

type ICondition[T any] interface {
	GetName() string
	Evaluate(T) (bool, error)
}
