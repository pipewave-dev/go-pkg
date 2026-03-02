package dynamodb

import (
	"reflect"
	"strings"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/rs/zerolog/log"
)

/*
# buildUpdateMixOperationExprFromStruct

generates a DynamoDB update expression for the given struct (or pointer of struct).
Each field's value is used as the attribute value in the update SET expression (new value)

 1. Format of tag is `dynamodbav:"attrName,OPERATION"`
    - Operator is one of the following: SET, ADD, DELETE, REMOVE, DIVE (only for struct)
    - If Operation is empty -> Default operation is SET
    - If Operation wrong value, this field will be skipped and notify a error.

 2. The field's tag `dynamodbav` are used as the attribute names in the update expression.

 3. The field values are used as the attribute values in the update expression.

 4. Any field that is nil or doesn't have tag `dynamodbav` is skipped.
*/
func buildUpdateMixOperationExprFromStruct(
	updateData interface{},
) expression.UpdateBuilder {
	return buildUpdateExprWithOperator(updateData, "MIX")
}

/*
# buildUpdateSETExprFromStruct

generates a DynamoDB update expression for the given struct (or pointer of struct).
Each field's value is used as the attribute value in the update SET expression (new value)

 1. Format of tag is `dynamodbav:"attrName"`

 2. The field's tag `dynamodbav` are used as the attribute names in the update expression.

 3. The field values are used as the attribute values in the update expression.

 4. Any field that is nil or doesn't have tag `dynamodbav` is skipped.
*/
func buildUpdateSETExprFromStruct(
	updateData interface{},
) expression.UpdateBuilder {
	return buildUpdateExprWithOperator(updateData, "SET")
}

/*
# buildUpdateADDExprFromStruct

generates a DynamoDB update expression for the given struct (or pointer of struct).
Each field's value is used as the attribute value in the update ADD expression (atomic increase value)

 1. Format of tag is `dynamodbav:"attrName"`

 2. The field's tag `dynamodbav` are used as the attribute names in the update expression.

 3. The field values are used as the attribute values in the update expression.

 4. Any field that is nil or doesn't have tag `dynamodbav` is skipped.

 5. Only Numberic types or Sets type are supported or will return dynamodb error when call dynamodb api.
*/
func buildUpdateADDExprFromStruct(
	updateData interface{},
) expression.UpdateBuilder {
	return buildUpdateExprWithOperator(updateData, "ADD")
}

/*
# buildUpdateDELETEExprFromStruct

generates a DynamoDB update expression for the given struct (or pointer of struct).
Each field's value is used as the attribute value in the update ADD expression (atomic increase value)

 1. Format of tag is `dynamodbav:"attrName"`

 2. The field's tag `dynamodbav` are used as the attribute names in the update expression.

 3. The field values are used as the attribute values in the update expression.

 4. Any field that is nil or doesn't have tag `dynamodbav` is skipped.

 5. Only Sets type are supported or will return dynamodb error when call dynamodb api.
*/
func buildUpdateDELETEExprFromStruct(
	updateData interface{},
) expression.UpdateBuilder {
	return buildUpdateExprWithOperator(updateData, "DELETE")
}

/*
# buildUpdateREMOVEExprFromStruct

generates a DynamoDB update expression for the given struct (or pointer of struct).
Each field's value is used as the attribute value in the update ADD expression (atomic increase value)

 1. Format of tag is `dynamodbav:"attrName"`

 2. The field's tag `dynamodbav` are used as the attribute names in the update expression.

 3. If the field values is not nil -> this attribute will be removed.

 4. Any field that is nil or doesn't have tag `dynamodbav` is skipped.
*/
func buildUpdateREMOVEExprFromStruct(
	updateData interface{},
) expression.UpdateBuilder {
	return buildUpdateExprWithOperator(updateData, "REMOVE")
}

func buildUpdateExprWithOperator(
	updateData interface{},
	operation string,
) expression.UpdateBuilder {

	// Iterate over the fields in the input struct.
	val := reflect.ValueOf(updateData)
	val = reflect.Indirect(val)

	typ := val.Type()
	update := &expression.UpdateBuilder{}
	for i := 0; i < val.NumField(); i++ {
		fieldVal := val.Field(i)

		// If the field value is nil, skip it.
		if fieldVal.IsNil() {
			continue
		}

		// Get the attribute name from the dynamodbav tag, if it exists.
		tag := typ.Field(i).Tag.Get("dynamodbav")
		if tag == "-" {
			continue
		}

		parts := strings.SplitN(tag, ",", 3)
		attrName := parts[0]
		if strings.Trim(attrName, " ") == "" {
			attrName = typ.Field(i).Name
		}
		operatorName := "SET" // Default if Operator is not defined
		if len(parts) >= 2 && parts[1] != "" {
			operatorName = parts[1]
		}

		fieldValue := reflect.Indirect(fieldVal).Interface()

		if reflect.Indirect(fieldVal).Kind() == reflect.Struct && operatorName == "DIVE" {
			fieldStructNested(fieldValue, operation, []string{attrName}, update)
		} else {
			// If operation == "MIX" (MIX case) we will get operation from tag
			var newOperation string
			if operation == "MIX" {
				newOperation = strings.ToUpper(operatorName)
			} else {
				newOperation = strings.ToUpper(operation)
			}
			// Add the field update to the expression.
			appendUpdateExpr(newOperation, update, []string{}, attrName, fieldValue)
		}
	}
	return *update
}

func fieldStructNested(
	updateData interface{},
	operation string,
	parentStacks []string,
	update *expression.UpdateBuilder,
) {
	// Iterate over the fields in the input struct.
	val := reflect.ValueOf(updateData)
	val = reflect.Indirect(val)

	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		fieldVal := val.Field(i)

		// If the field value is nil, skip it.
		if fieldVal.IsNil() {
			continue
		}

		// Get the attribute name from the dynamodbav tag, if it exists.
		tag := typ.Field(i).Tag.Get("dynamodbav")
		if tag == "-" {
			continue
		}

		parts := strings.SplitN(tag, ",", 3)
		attrName := parts[0]
		if strings.Trim(attrName, " ") == "" {
			attrName = typ.Field(i).Name
		}
		operatorName := "SET" // Default if Operator is not defined
		if len(parts) >= 2 && parts[1] != "" {
			operatorName = parts[1]
		}

		fieldValue := reflect.Indirect(fieldVal).Interface()

		if reflect.Indirect(fieldVal).Kind() == reflect.Struct && operatorName == "DIVE" {
			fieldStructNested(fieldValue, operation, append(parentStacks, attrName), update)
			continue
		}

		// If operation == "MIX" (MIX case) we will get operation from tag
		var newOperation string
		if operation == "MIX" {
			newOperation = strings.ToUpper(operatorName)
		} else {
			newOperation = strings.ToUpper(operation)
		}
		// Add the field update to the expression.
		appendUpdateExpr(newOperation, update, parentStacks, attrName, fieldValue)

	}
}

func appendUpdateExpr(
	operation string,
	update *expression.UpdateBuilder,
	parentStacks []string,
	attrName string,
	fieldValue any,
) {
	var fullAttributeString string
	if len(parentStacks) > 0 {
		fullAttributeString = strings.Join(
			append(parentStacks, attrName), ".")
	} else {
		fullAttributeString = attrName
	}

	// 3 special fields: updated_at, created_at, deleted_at always use SET operation
	if attrName == "updated_at" || attrName == "created_at" || attrName == "deleted_at" {
		*update = update.Set(expression.Name(fullAttributeString), expression.Value(fieldValue))
		return
	}

	// Add the field update to the expression.
	switch operation {
	case "SET":
		*update = update.Set(expression.Name(fullAttributeString), expression.Value(fieldValue))
	case "ADD":
		*update = update.Add(expression.Name(fullAttributeString), expression.Value(fieldValue))
	case "DELETE":
		*update = update.Delete(expression.Name(fullAttributeString), expression.Value(fieldValue))
	case "REMOVE":
		*update = update.Remove(expression.Name(fullAttributeString))
	default:
		// skip if send wrong operation
		log.Error().
			Msgf(" buildUpdateExprWithOperator send wrong operation: [%s]. "+
				"This must be not happened. Please recheck your code in", operation)
	}
}
