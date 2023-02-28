package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/infinitybotlist/eureka/crypto"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ctx = context.Background()

// A link is any extra link, needed for user insert
type Link struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type TeamPermission string // TeamPermission is a permission that a team can have as of creation of this tool

const (
	TeamPermissionUndefined                 TeamPermission = ""
	TeamPermissionEditBotSettings           TeamPermission = "EDIT_BOT_SETTINGS"
	TeamPermissionAddNewBots                TeamPermission = "ADD_NEW_BOTS"
	TeamPermissionResubmitBots              TeamPermission = "RESUBMIT_BOTS"
	TeamPermissionCertifyBots               TeamPermission = "CERTIFY_BOTS"
	TeamPermissionResetBotTokens            TeamPermission = "RESET_BOT_TOKEN"
	TeamPermissionEditBotWebhooks           TeamPermission = "EDIT_BOT_WEBHOOKS"
	TeamPermissionTestBotWebhooks           TeamPermission = "TEST_BOT_WEBHOOKS"
	TeamPermissionSetBotVanity              TeamPermission = "SET_BOT_VANITY"
	TeamPermissionEditTeamNameAvatar        TeamPermission = "EDIT_TEAM_NAME_AVATAR"
	TeamPermissionAddTeamMembers            TeamPermission = "ADD_TEAM_MEMBERS"
	TeamPermissionRemoveTeamMembers         TeamPermission = "REMOVE_TEAM_MEMBERS"
	TeamPermissionEditTeamMemberPermissions TeamPermission = "EDIT_TEAM_MEMBER_PERMISSIONS"
	TeamPermissionDeleteBots                TeamPermission = "DELETE_BOTS"
	TeamPermissionOwner                     TeamPermission = "OWNER"
)

// Default perms to use
var ownerPerms []TeamPermission = []TeamPermission{TeamPermissionOwner}
var addOwnerPerms []TeamPermission = []TeamPermission{
	TeamPermissionEditBotSettings,
	TeamPermissionAddNewBots,
	TeamPermissionResubmitBots,
	TeamPermissionCertifyBots,
	TeamPermissionResetBotTokens,
	TeamPermissionEditBotWebhooks,
	TeamPermissionTestBotWebhooks,
}

func main() {
	pool, err := pgxpool.New(ctx, "postgresql:///infinity")

	if err != nil {
		panic(err)
	}

	defer pool.Close()

	var count int

	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM bots WHERE cardinality(additional_owners) > 0").Scan(&count)

	if err != nil {
		panic(err)
	}

	fmt.Println("Going to update", count, "bots to teams")

	tx, err := pool.Begin(ctx)

	if err != nil {
		panic(err)
	}

	defer tx.Rollback(ctx)

	rows, err := pool.Query(ctx, "SELECT bot_id, queue_name, queue_avatar, owner, additional_owners FROM bots WHERE cardinality(additional_owners) > 0")

	if err != nil {
		panic(err)
	}

	defer rows.Close()

	for rows.Next() {
		var botId string
		var queueName string
		var queueAvatar string
		var owner string
		var additionalOwners []string

		err = rows.Scan(&botId, &queueName, &queueAvatar, &owner, &additionalOwners)

		if err != nil {
			panic(err)
		}

		fmt.Println("Updating", botId, "("+queueName+") to team", owner, "with additional owners", additionalOwners)

		// Create new team for this bot
		var teamId pgtype.UUID
		err = tx.QueryRow(ctx, "INSERT INTO teams (name, avatar) VALUES ($1, $2) RETURNING id", queueName, queueAvatar).Scan(&teamId)

		if err != nil {
			panic(err)
		}

		// Add the user to the team
		_, err = tx.Exec(ctx, "INSERT INTO team_members (team_id, user_id, perms) VALUES ($1, $2, $3)", teamId, owner, ownerPerms)

		if err != nil {
			panic(err)
		}

		for _, additionalOwner := range additionalOwners {
			if len(strings.ReplaceAll(additionalOwner, " ", "")) == 0 {
				panic("Invalid additional owner")
			}

			// Check that owner is on the database
			var count int

			err = tx.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE user_id = $1", additionalOwner).Scan(&count)

			if err != nil {
				panic(err)
			}

			if count == 0 {
				// Add the user
				fmt.Println("Adding user", additionalOwner, "to the database")

				// Create user
				apiToken := crypto.RandString(128)
				_, err = tx.Exec(
					ctx,
					"INSERT INTO users (user_id, api_token, extra_links, staff, developer, certified) VALUES ($1, $2, $3, false, false, false)",
					additionalOwner,
					apiToken,
					[]Link{},
				)

				if err != nil {
					panic(err)
				}
			}

			_, err = tx.Exec(ctx, "INSERT INTO team_members (team_id, user_id, perms) VALUES ($1, $2, $3)", teamId, additionalOwner, addOwnerPerms)

			if err != nil {
				panic(err)
			}
		}

		// Update the bot to use the team
		_, err = tx.Exec(ctx, "UPDATE bots SET owner = NULL, team_owner = $1, additional_owners = '{}' WHERE bot_id = $2", teamId, botId)

		if err != nil {
			panic(err)
		}
	}

	err = tx.Commit(ctx)

	if err != nil {
		panic(err)
	}
}
