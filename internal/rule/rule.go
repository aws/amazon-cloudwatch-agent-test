package rule

type Rule struct {
	Conditions []ICondition
}

func (r *Rule) Evaluate(permission string) bool {
	for _, c := range r.Conditions {
		if !c.Evaluate(permission) {
			return false
		}
	}
	return true
}

type ICondition interface {
	Evaluate(string) bool
}

type ExactMatch struct {
	ExpectedValue string
}

func (e *ExactMatch) Evaluate(permission string) bool {
	return permission == e.ExpectedValue
}

var _ ICondition = (*ExactMatch)(nil)
