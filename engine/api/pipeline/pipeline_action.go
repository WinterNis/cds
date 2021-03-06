package pipeline

import (
	"fmt"

	"github.com/go-gorp/gorp"

	"github.com/ovh/cds/engine/api/action"
	"github.com/ovh/cds/engine/log"
	"github.com/ovh/cds/sdk"
)

// DeletePipelineActionByStage Delete all action from a stage
func DeletePipelineActionByStage(db gorp.SqlExecutor, stageID int64, userID int64) error {
	pipelineActionsID, errSelect := selectAllPipelineActionID(db, stageID)
	if errSelect != nil {
		return errSelect
	}

	// For all pipeline_action in stage
	for i := range pipelineActionsID {
		var id int64
		var actionType string
		// Fetch id and type of action linked to pipeline_action so we can delete it if it's a joined action
		query := `SELECT action.id, action.type FROM action JOIN pipeline_action ON pipeline_action.action_id = action.id WHERE pipeline_action.id = $1`
		if err := db.QueryRow(query, pipelineActionsID[i]).Scan(&id, &actionType); err != nil {
			return err
		}

		// Delete pipeline_action
		query = `DELETE FROM pipeline_action WHERE id = $1`
		if _, err := db.Exec(query, pipelineActionsID[i]); err != nil {
			return err
		}

		// Then if action is a Joined Action delete action as well
		if actionType != sdk.JoinedAction {
			continue
		}
		log.Info("DeletePipelineActionByStage> Deleting action %d\n", id)
		if err := action.DeleteAction(db, id, userID); err != nil {
			return err
		}
	}

	return nil
}

func selectAllPipelineActionID(db gorp.SqlExecutor, pipelineStageID int64) ([]int64, error) {
	var pipelineActionIDs []int64
	query := `SELECT id FROM "pipeline_action"
	 		  WHERE pipeline_stage_id = $1`
	rows, err := db.Query(query, pipelineStageID)
	if err != nil {
		return pipelineActionIDs, err
	}
	defer rows.Close()

	for rows.Next() {
		var pipelineActionID int64
		err = rows.Scan(&pipelineActionID)
		if err != nil {
			return pipelineActionIDs, err
		}
		pipelineActionIDs = append(pipelineActionIDs, pipelineActionID)
	}
	return pipelineActionIDs, nil
}

// InsertJob  Insert a new Job ( pipeline_action + joinedAction )
func InsertJob(db gorp.SqlExecutor, job *sdk.Job, stageID int64, pip *sdk.Pipeline) error {
	// Insert Joined Action
	job.Action.Type = sdk.JoinedAction
	job.Action.Enabled = true
	job.Enabled = true
	log.Debug("InsertJob> Insert Action %s on pipeline %s with %d children", job.Action.Name, pip.Name, len(job.Action.Actions))
	if err := action.InsertAction(db, &job.Action, false); err != nil {
		return err
	}

	// Create Stage if needed
	var stage *sdk.Stage
	if stageID == 0 {
		stage = &sdk.Stage{
			Name:       fmt.Sprintf("Stage %d", len(pip.Stages)+1),
			PipelineID: pip.ID,
			BuildOrder: len(pip.Stages) + 1,
			Enabled:    true,
		}
		log.Debug("InsertJob> Creating stage %s on pipeline %s", stage.Name, pip.Name)
		if err := InsertStage(db, stage); err != nil {
			return fmt.Errorf("Cannot InsertStage on pipeline %d> %s", pip.ID, err)
		}
	} else {
		//Else load the stage
		var errLoad error
		stage, errLoad = LoadStage(db, pip.ID, stageID)
		if errLoad != nil {
			return errLoad
		}
		log.Debug("InsertJob> Load existing stage %s on pipeline %s", stage.Name, pip.Name)
	}
	job.PipelineStageID = stage.ID

	// Create pipeline action
	query := `INSERT INTO pipeline_action (pipeline_stage_id, action_id, enabled) VALUES ($1, $2, $3) RETURNING id`
	if err := db.QueryRow(query, job.PipelineStageID, job.Action.ID, job.Enabled).Scan(&job.PipelineActionID); err != nil {
		return err
	}
	return nil
}

// UpdateJob  updates the job by actionData.PipelineActionID and actionData.ID
func UpdateJob(db gorp.SqlExecutor, job *sdk.Job, userID int64) error {
	clearJoinedAction, err := action.LoadActionByID(db, job.Action.ID)
	if err != nil {
		return err
	}

	if clearJoinedAction.Type != sdk.JoinedAction {
		return sdk.ErrForbidden
	}

	query := `UPDATE pipeline_action set action_id=$1, pipeline_stage_id=$2, enabled=$4  WHERE id=$3`
	_, err = db.Exec(query, job.Action.ID, job.PipelineStageID, job.PipelineActionID, job.Enabled)
	if err != nil {
		return err
	}
	return action.UpdateActionDB(db, &job.Action, userID)
}

// DeleteJob Delete a job ( action + pipeline_action )
func DeleteJob(db gorp.SqlExecutor, job sdk.Job, userID int64) error {
	return action.DeleteAction(db, job.Action.ID, userID)
}

// UpdatePipelineAction Update an action in a pipeline
func UpdatePipelineAction(db gorp.SqlExecutor, job sdk.Job) error {
	query := `UPDATE pipeline_action set action_id=$1, pipeline_stage_id=$2, enabled=$4  WHERE id=$3`

	_, err := db.Exec(query, job.Action.ID, job.PipelineStageID, job.PipelineActionID, job.Enabled)
	if err != nil {
		return err
	}

	return nil
}

// DeletePipelineAction Delete an action in a pipeline
func DeletePipelineAction(db gorp.SqlExecutor, pipelineActionID int64) error {

	// Delete pipelineAction by buildOrder
	query := `DELETE FROM pipeline_action WHERE id = $1`
	_, err := db.Exec(query, pipelineActionID)
	if err != nil {
		return err
	}

	return nil
}
