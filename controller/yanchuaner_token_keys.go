/*
Copyright (C) 2026 Yanchuaner Ecosystem Contributors

This file is original Yanchuaner Ecosystem work and is distributed under
the GNU Affero General Public License version 3 or later with this repository.
*/
package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func yanCoreTokenUserId(c *gin.Context) (int, bool) {
	if !common.GetEnvOrDefaultBool("YANCHUANER_SUBJECT_GRANTS_ENABLED", false) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "YanCore subject grants are disabled."})
		return 0, false
	}
	claims, err := model.ParseSubjectGrantForAudience(bearerValue(c.GetHeader("Authorization")), "yanchuaner-ai")
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "YanCore subject grant is invalid."})
		return 0, false
	}
	userId := 0
	if strings.HasPrefix(claims.Subject, "yc_user_") {
		userId, _ = strconv.Atoi(strings.TrimPrefix(claims.Subject, "yc_user_"))
	}
	if userId <= 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "YanCore subject grant is invalid."})
		return 0, false
	}
	return userId, true
}

// YanCoreTokenList lists the subject's own tokens without exposing any key
// material. The full key never appears again after creation.
func YanCoreTokenList(c *gin.Context) {
	userId, ok := yanCoreTokenUserId(c)
	if !ok {
		return
	}
	pageInfo := common.GetPageQuery(c)
	tokens, err := model.GetAllUserTokens(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]*model.Token, 0, len(tokens))
	for _, token := range tokens {
		items = append(items, buildMaskedTokenResponse(token))
	}
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

// YanCoreTokenCreate reuses AddToken so key hashing, budget, model limits and
// future policy creation stay identical to the New API path.
func YanCoreTokenCreate(c *gin.Context) {
	userId, ok := yanCoreTokenUserId(c)
	if !ok {
		return
	}
	c.Set("id", userId)
	AddToken(c)
}

// YanCoreTokenDelete deletes only the subject's own token.
func YanCoreTokenDelete(c *gin.Context) {
	userId, ok := yanCoreTokenUserId(c)
	if !ok {
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "token id is invalid"})
		return
	}
	if err := model.DeleteTokenById(id, userId); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}
