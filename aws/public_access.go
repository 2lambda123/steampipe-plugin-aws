package aws

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws/arn"

	"github.com/turbot/go-kit/helpers"
)

var (
	// AWS condition operators to be checked for the trusted access
	conditionOperatorsToCheck = []string{
		"ArnEquals",
		"ArnEqualsIfExists",
		"ArnLike",
		"ArnLikeIfExists",
		"StringEquals",
		"StringEqualsIfExists",
		"StringEqualsIgnoreCase",
		"StringEqualsIgnoreCaseIfExists",
		"StringLike",
		"StringLikeIfExists",
	}
	// AWS Global Keys to be checked for the trusted access for AWS Principal
	trustedAWSPrincipalConditionKeys = []string{
		"aws:principalaccount",
		"aws:principalarn",
		"aws:principalorgid",
		"aws:principalorgpaths", //["o-a1b2c3d4e5/*"]  , ["o-a1b2c3d4e5/r-ab12/ou-ab12-11111111/ou-ab12-22222222/ou-*"]
		/*
					"Condition" : { "ForAnyValue:StringEquals" : {
			     "aws:PrincipalOrgPaths":["o-a1b2c3d4e5/r-ab12/ou-ab12-11111111/ou-ab12-22222222/"]
					}}

					"Condition" : { "ForAnyValue:StringLike" : {
			     "aws:PrincipalOrgPaths":["o-a1b2c3d4e5/r-ab12/ou-ab12-11111111/ou-ab12-22222222/*"]
					}}

					"Condition" : { "ForAnyValue:StringLike" : {
			     "aws:PrincipalOrgPaths":["o-a1b2c3d4e5/r-ab12/ou-ab12-11111111/ou-ab12-22222222/*"]
					}}

					"Condition" : { "ForAnyValue:StringLike" : {
			     "aws:PrincipalOrgPaths":["o-a1b2c3d4e5/*"]
					}}
					{
						"ForAnyValue:StringLike": {
							"aws:PrincipalOrgPaths": [
								"o-a1b2c3d4e5/r-ab12/ou-ab12-33333333/*",
								"o-a1b2c3d4e5/r-ab12/ou-ab12-22222222/*"
							]
						}
					}
		*/
	}
	// AWS Global Keys to be checked for the trusted access for Service Principal
	trustedServicePrincipalConditionKeys = []string{
		"aws:sourcearn",
		"aws:sourceaccount", // SourceAccount is used for giving IAM roles access from an account to the topic.
		"aws:sourceowner",   // SourceOwner is used for giving access to other AWS Services from a specific account
	}
)

type PolicyEvaluation struct {
	// Policy                               Policy   `json:"policy"`
	AccessLevel                         string   `json:"access_level"`
	AllowedOrganizationIds              []string `json:"allowed_organization_ids"`
	AllowedPrincipals                   []string `json:"allowed_principals"`
	AllowedPrincipalAccountIds          []string `json:"allowed_principal_account_ids"`
	AllowedPrincipalFederatedIdentities []string `json:"allowed_principal_federated_identities"`
	AllowedPrincipalServices            []string `json:"allowed_principal_services"`
	IsPublic                            bool     `json:"is_public"`
	PublicAccessLevels                  []string `json:"public_access_levels"`
	PublicStatementIds                  []string `json:"public_statement_ids"`
}

func (policy *Policy) EvaluatePolicy() (*PolicyEvaluation, error) {

	evaluation := PolicyEvaluation{}

	if policy.Statements == nil {
		return &evaluation, nil
	}

	for index, stmt := range policy.Statements {
		public := stmt.EvaluateStatement(&evaluation)
		if public {
			evaluation.IsPublic = true
			if stmt.Sid == "" {
				evaluation.PublicStatementIds = append(evaluation.PublicStatementIds, fmt.Sprintf("Statement[%s]", strconv.Itoa(index+1)))
			} else {
				evaluation.PublicStatementIds = append(evaluation.PublicStatementIds, stmt.Sid)
			}
		}
	}

	evaluation.AllowedPrincipalAccountIds = StringSliceDistinct(evaluation.AllowedPrincipalAccountIds)
	accountIds := []string{}
	for _, item := range StringSliceDistinct(evaluation.AllowedPrincipals) {
		if arn.IsARN(item) {
			awsARN, _ := arn.Parse(item)
			if awsARN.AccountID != "" {
				accountIds = append(accountIds, awsARN.AccountID)
			}
		} else {
			// TODO - Should we add principals which doesn't have account ids
			accountIds = append(accountIds, item)
		}
	}
	evaluation.AllowedPrincipalAccountIds = accountIds

	// Add all types of principals into allowed principals
	evaluation.AllowedPrincipals = StringSliceDistinct(evaluation.AllowedPrincipals)
	evaluation.AllowedPrincipals = append(evaluation.AllowedPrincipals, evaluation.AllowedPrincipalServices...)
	evaluation.AllowedPrincipals = StringSliceDistinct(append(evaluation.AllowedPrincipals, evaluation.AllowedPrincipalFederatedIdentities...))

	evaluation.AllowedPrincipalFederatedIdentities = StringSliceDistinct(evaluation.AllowedPrincipalFederatedIdentities)
	evaluation.AllowedOrganizationIds = StringSliceDistinct(evaluation.AllowedOrganizationIds)
	evaluation.AllowedPrincipalServices = StringSliceDistinct(evaluation.AllowedPrincipalServices)
	evaluation.PublicAccessLevels = StringSliceDistinct(evaluation.PublicAccessLevels)
	evaluation.PublicStatementIds = StringSliceDistinct(evaluation.PublicStatementIds)

	return &evaluation, nil
}

func (stmt *Statement) EvaluateStatement(evaluation *PolicyEvaluation) bool {
	// Check for the deny statements separately
	if stmt.Effect == "Deny" {
		// TODO
		return stmt.DenyStatementEvaluation(evaluation)
	}

	// Check for the allowed statement
	if stmt.NotAction != nil {
		return true
	}

	var awsPrincipals, servicePrincipals, federatedPrincipals []string
	var hasPublicPrincipal = false
	var isPublic = false
	if stmt.Principal != nil {
		if data, ok := stmt.Principal["AWS"]; ok {
			awsPrincipals = data.([]string)
			evaluation.AllowedPrincipals = append(evaluation.AllowedPrincipals, awsPrincipals...)
		}
		if data, ok := stmt.Principal["Service"]; ok {
			servicePrincipals = data.([]string)
			evaluation.AllowedPrincipalServices = append(evaluation.AllowedPrincipalServices, servicePrincipals...)
		}
		if data, ok := stmt.Principal["Federated"]; ok {
			federatedPrincipals = data.([]string)
			evaluation.AllowedPrincipalFederatedIdentities = append(evaluation.AllowedPrincipalFederatedIdentities, federatedPrincipals...)
		}
	}
	if helpers.StringSliceContains(awsPrincipals, "*") {
		hasPublicPrincipal = true
		isPublic = true
	}

	if stmt.Condition != nil {
		// log.Println("[INFO] AM I REACHING HERE")
		for key, value := range stmt.Condition {
			// hasAnyValuePrefix := CheckForAnyValuePrefix(key)
			// hasAllValuesPrefix := CheckForAllValuesPrefix(key)
			hasIfExistsSuffix := CheckIfExistsSuffix(key)
			// log.Println("[INFO] operator key:", key)

			// if helpers.StringSliceContains(conditionOperatorsToCheck, key) {

			// }
			// log.Println("[INFO] operator value:", value)
			// log.Printf("[INFO] operator value type: %T\n", value)
			if conditiionOperatorValueMap, ok := value.(map[string]interface{}); ok {
				// log.Println("[INFO] operator key:", key)
				for conditionKey, conditionValue := range conditiionOperatorValueMap {

					// Check if the Principals contain * principals, in that case it is public but if there is a restriction using conditions then it will not remain public
					if hasPublicPrincipal {
						if hasAWSPrincipalConditionKey(conditionKey) && !hasIfExistsSuffix {
							isPublic = false
						}
						if hasServicePrincipalConditionKey(conditionKey) && !hasIfExistsSuffix {
							isPublic = false
						}
					}
					// If the policy have principal org or path to org need to add that in the evaluation
					if helpers.StringSliceContains([]string{"aws:principalorgid", "aws:principalorgpaths"}, strings.ToLower(conditionKey)) {
						if val, ok := conditionValue.([]string); ok {
							evaluation.AllowedOrganizationIds = append(evaluation.AllowedOrganizationIds, val...)
						}
					}
				}
			}

		}
	}
	return isPublic
}

func (stmt *Statement) DenyStatementEvaluation(evaluation *PolicyEvaluation) bool {
	return false
}

/*
[
  {
    "Version": "2012-10-17",
    "Statement": [
      {
        "Effect": "Allow",
        "Action": "dynamodb:GetItem",
        "Resource": "arn:aws:dynamodb:*:*:table/Thread",
        "Condition": {
          "ForAllValues:StringEquals": {
            "dynamodb:Attributes": ["ID", "Message", "Tags"]
          }
        }
      }
    ]
  },
  {
    "Version": "2012-10-17",
    "Statement": {
      "Effect": "Deny",
      "Action": "dynamodb:PutItem",
      "Resource": "arn:aws:dynamodb:*:*:table/Thread",
      "Condition": {
        "ForAnyValue:StringEquals": {
          "dynamodb:Attributes": ["ID", "PostDateTime"]
        }
      }
    }
  }
]
*/
// https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_multi-value-conditions.html
func CheckForAnyValuePrefix(key string) bool {
	return strings.HasPrefix(key, "ForAnyValue")
}
func CheckForAllValuesPrefix(key string) bool {
	return strings.HasPrefix(key, "ForAllValues")
}
func CheckIfExistsSuffix(key string) bool {
	return strings.HasSuffix(key, "IfExists")
}
func hasAWSPrincipalConditionKey(conditionKey string) bool {
	return helpers.StringSliceContains(trustedAWSPrincipalConditionKeys, strings.ToLower(conditionKey))
}
func hasServicePrincipalConditionKey(conditionKey string) bool {
	return helpers.StringSliceContains(trustedServicePrincipalConditionKeys, strings.ToLower(conditionKey))
}

// StringSliceDistinct returns a slice with the unique elements the input string slice
func StringSliceDistinct(slice []string) []string {
	var res []string
	countMap := make(map[string]int)
	for _, item := range slice {
		countMap[item]++
	}
	for item := range countMap {
		res = append(res, item)
	}
	return res
}