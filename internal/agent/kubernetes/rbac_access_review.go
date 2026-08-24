package kubernetes

import (
	"context"
	"time"

	domainresource "github.com/opensoha/soha-agent/internal/domain/resource"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Client) ReviewSubjectAccess(ctx context.Context, input domainresource.SubjectAccessReviewInput) (domainresource.SubjectAccessReviewResult, error) {
	user, groups := accessReviewIdentity(input.Subject)
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result := domainresource.SubjectAccessReviewResult{Subject: input.Subject, Decisions: make([]domainresource.AccessReviewDecision, 0, len(input.Checks))}
	for _, check := range input.Checks {
		review, err := c.typed.AuthorizationV1().SubjectAccessReviews().Create(queryCtx, &authorizationv1.SubjectAccessReview{
			Spec: authorizationv1.SubjectAccessReviewSpec{
				User: user, Groups: groups,
				ResourceAttributes: &authorizationv1.ResourceAttributes{
					Verb: check.Verb, Group: check.Group, Resource: check.Resource,
					Namespace: check.Namespace, Name: check.Name,
				},
			},
		}, metav1.CreateOptions{})
		if err != nil {
			return domainresource.SubjectAccessReviewResult{}, err
		}
		result.Decisions = append(result.Decisions, domainresource.AccessReviewDecision{
			Check: check, Allowed: review.Status.Allowed, Denied: review.Status.Denied,
			Reason: review.Status.Reason, EvaluationError: review.Status.EvaluationError,
		})
	}
	return result, nil
}

func accessReviewIdentity(subject domainresource.AccessReviewSubject) (string, []string) {
	switch subject.Kind {
	case "Group":
		return "", []string{subject.Name}
	case "ServiceAccount":
		return "system:serviceaccount:" + subject.Namespace + ":" + subject.Name, []string{
			"system:serviceaccounts", "system:serviceaccounts:" + subject.Namespace, "system:authenticated",
		}
	default:
		return subject.Name, nil
	}
}
