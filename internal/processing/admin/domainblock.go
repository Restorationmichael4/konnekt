// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package admin

import (
	"context"
	"errors"
	"fmt"

	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"code.superseriousbusiness.org/gotosocial/internal/text"
)

func (p *Processor) createDomainBlock(
	ctx context.Context,
	adminAcct *gtsmodel.Account,
	domain string,
	obfuscate bool,
	publicComment string,
	privateComment string,
	subscriptionID string,
) (*apimodel.DomainPermission, string, gtserror.WithCode) {
	// Check if a block already exists for this domain.
	domainBlock, err := p.state.DB.GetDomainBlock(ctx, domain)
	if err != nil && !errors.Is(err, db.ErrNoEntries) {
		// Something went wrong in the DB.
		err = gtserror.Newf("db error getting domain block %s: %w", domain, err)
		return nil, "", gtserror.NewErrorInternalError(err)
	}

	if domainBlock == nil {
		// No block exists yet, create it.
		domainBlock = &gtsmodel.DomainBlock{
			ID:                 id.NewULID(),
			Domain:             domain,
			CreatedByAccountID: adminAcct.ID,
			PrivateComment:     text.StripHTMLFromText(privateComment),
			PublicComment:      text.StripHTMLFromText(publicComment),
			Obfuscate:          &obfuscate,
			SubscriptionID:     subscriptionID,
		}

		// Insert the new block into the database.
		if err := p.state.DB.PutDomainBlock(ctx, domainBlock); err != nil {
			err = gtserror.Newf("db error putting domain block %s: %w", domain, err)
			return nil, "", gtserror.NewErrorInternalError(err)
		}
	}

	// Run admin action to process
	// side effects of block.
	action := &gtsmodel.AdminAction{
		ID:             id.NewULID(),
		TargetCategory: gtsmodel.AdminActionCategoryDomain,
		TargetID:       domain,
		Type:           gtsmodel.AdminActionSuspend,
		AccountID:      adminAcct.ID,
		Text:           domainBlock.PrivateComment,
	}

	if errWithCode := p.state.AdminActions.Run(
		ctx,
		action,
		p.state.AdminActions.DomainBlockF(action.ID, domainBlock),
	); errWithCode != nil {
		return nil, action.ID, errWithCode
	}

	apiDomainBlock, errWithCode := p.apiDomainPerm(ctx, domainBlock, false)
	if errWithCode != nil {
		return nil, action.ID, errWithCode
	}

	return apiDomainBlock, action.ID, nil
}

func (p *Processor) updateDomainBlock(
	ctx context.Context,
	domainBlockID string,
	obfuscate *bool,
	publicComment *string,
	privateComment *string,
	subscriptionID *string,
) (*apimodel.DomainPermission, gtserror.WithCode) {
	domainBlock, err := p.state.DB.GetDomainBlockByID(ctx, domainBlockID)
	if err != nil {
		if !errors.Is(err, db.ErrNoEntries) {
			// Real error.
			err = gtserror.Newf("db error getting domain block: %w", err)
			return nil, gtserror.NewErrorInternalError(err)
		}

		// There are just no entries for this ID.
		err = fmt.Errorf("no domain block entry exists with ID %s", domainBlockID)
		return nil, gtserror.NewErrorNotFound(err, err.Error())
	}

	var columns []string
	if obfuscate != nil {
		domainBlock.Obfuscate = obfuscate
		columns = append(columns, "obfuscate")
	}
	if publicComment != nil {
		domainBlock.PublicComment = *publicComment
		columns = append(columns, "public_comment")
	}
	if privateComment != nil {
		domainBlock.PrivateComment = *privateComment
		columns = append(columns, "private_comment")
	}
	if subscriptionID != nil {
		domainBlock.SubscriptionID = *subscriptionID
		columns = append(columns, "subscription_id")
	}

	// Update the domain block.
	if err := p.state.DB.UpdateDomainBlock(ctx, domainBlock, columns...); err != nil {
		err = gtserror.Newf("db error updating domain block: %w", err)
		return nil, gtserror.NewErrorInternalError(err)
	}

	return p.apiDomainPerm(ctx, domainBlock, false)
}

func (p *Processor) deleteDomainBlock(
	ctx context.Context,
	adminAcct *gtsmodel.Account,
	domainBlockID string,
) (*apimodel.DomainPermission, string, gtserror.WithCode) {
	domainBlock, err := p.state.DB.GetDomainBlockByID(ctx, domainBlockID)
	if err != nil {
		if !errors.Is(err, db.ErrNoEntries) {
			// Real error.
			err = gtserror.Newf("db error getting domain block: %w", err)
			return nil, "", gtserror.NewErrorInternalError(err)
		}

		// There are just no entries for this ID.
		err = fmt.Errorf("no domain block entry exists with ID %s", domainBlockID)
		return nil, "", gtserror.NewErrorNotFound(err, err.Error())
	}

	// Prepare the domain block to return, *before* the deletion goes through.
	apiDomainBlock, errWithCode := p.apiDomainPerm(ctx, domainBlock, false)
	if errWithCode != nil {
		return nil, "", errWithCode
	}

	// Delete the original domain block.
	if err := p.state.DB.DeleteDomainBlock(ctx, domainBlock.Domain); err != nil {
		err = gtserror.Newf("db error deleting domain block: %w", err)
		return nil, "", gtserror.NewErrorInternalError(err)
	}

	// Run admin action to process
	// side effects of unblock.
	action := &gtsmodel.AdminAction{
		ID:             id.NewULID(),
		TargetCategory: gtsmodel.AdminActionCategoryDomain,
		TargetID:       domainBlock.Domain,
		Type:           gtsmodel.AdminActionUnsuspend,
		AccountID:      adminAcct.ID,
	}

	if errWithCode := p.state.AdminActions.Run(
		ctx,
		action,
		p.state.AdminActions.DomainUnblockF(action.ID, domainBlock),
	); errWithCode != nil {
		return nil, action.ID, errWithCode
	}

	return apiDomainBlock, action.ID, nil
}
