/*
Copyright 2022 The Magma Authors.

This source code is licensed under the BSD-style license found in the
LICENSE file in the root directory of this source tree.

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package storage_test

import (
	"fmt"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/stretchr/testify/suite"

	"magma/dp/cloud/go/services/dp/storage"
	"magma/dp/cloud/go/services/dp/storage/db"
	"magma/dp/cloud/go/services/dp/storage/dbtest"
	"magma/orc8r/cloud/go/sqorc"
	"magma/orc8r/lib/go/merrors"
)

func TestCbsdManager(t *testing.T) {
	suite.Run(t, &CbsdManagerTestSuite{})
}

type CbsdManagerTestSuite struct {
	suite.Suite
	cbsdManager     storage.CbsdManager
	resourceManager dbtest.ResourceManager
	enumMaps        map[string]map[string]int64
}

func (s *CbsdManagerTestSuite) SetupSuite() {
	builder := sqorc.GetSqlBuilder()
	errorChecker := sqorc.SQLiteErrorChecker{}
	database, err := sqorc.Open("sqlite3", ":memory:")
	s.Require().NoError(err)
	s.cbsdManager = storage.NewCbsdManager(database, builder, errorChecker)
	s.resourceManager = dbtest.NewResourceManager(s.T(), database, builder)
	err = s.resourceManager.CreateTables(
		&storage.DBCbsdState{},
		&storage.DBCbsd{},
		&storage.DBActiveModeConfig{},
		&storage.DBGrantState{},
		&storage.DBGrant{},
	)
	s.Require().NoError(err)
	err = s.resourceManager.InsertResources(
		db.NewExcludeMask("id"),
		&storage.DBCbsdState{Name: db.MakeString("unregistered")},
		&storage.DBCbsdState{Name: db.MakeString("registered")},
		&storage.DBGrantState{Name: db.MakeString("idle")},
		&storage.DBGrantState{Name: db.MakeString("granted")},
		&storage.DBGrantState{Name: db.MakeString("authorized")},
	)
	s.Require().NoError(err)
	s.enumMaps = map[string]map[string]int64{}
	for _, model := range []db.Model{
		&storage.DBCbsdState{},
		&storage.DBGrantState{},
	} {
		table := model.GetMetadata().Table
		s.enumMaps[table] = s.getNameIdMapping(model)
	}

}

func (s *CbsdManagerTestSuite) TearDownTest() {
	err := s.resourceManager.DropResources(
		&storage.DBCbsd{},
		&storage.DBActiveModeConfig{},
		&storage.DBGrant{},
	)
	s.Require().NoError(err)
}

const (
	someNetwork  = "some_network"
	otherNetwork = "other_network_id"
)

func (s *CbsdManagerTestSuite) TestCreateCbsd() {
	err := s.cbsdManager.CreateCbsd(someNetwork, getBaseCbsd())
	s.Require().NoError(err)

	err = s.resourceManager.InTransaction(func() {
		actual, err := db.NewQuery().
			WithBuilder(s.resourceManager.GetBuilder()).
			From(&storage.DBCbsd{}).
			Select(db.NewExcludeMask("id", "state_id")).
			Join(db.NewQuery().
				From(&storage.DBCbsdState{}).
				On(db.On(storage.CbsdTable, "state_id", storage.CbsdStateTable, "id")).
				Select(db.NewIncludeMask("name"))).
			Where(sq.Eq{"cbsd_serial_number": "some_serial_number"}).
			Fetch()
		s.Require().NoError(err)

		cbsd := getBaseCbsd()
		cbsd.NetworkId = db.MakeString(someNetwork)
		cbsd.GrantAttempts = db.MakeInt(0)
		cbsd.IsDeleted = db.MakeBool(false)
		cbsd.ShouldDeregister = db.MakeBool(false)
		expected := []db.Model{
			cbsd,
			&storage.DBCbsdState{Name: db.MakeString("unregistered")},
		}
		s.Assert().Equal(expected, actual)

		actual, err = db.NewQuery().
			WithBuilder(s.resourceManager.GetBuilder()).
			From(&storage.DBActiveModeConfig{}).
			Select(db.NewIncludeMask()).
			Join(db.NewQuery().
				From(&storage.DBCbsdState{}).
				On(db.On(storage.CbsdStateTable, "id", storage.ActiveModeConfigTable, "desired_state_id")).
				Select(db.NewIncludeMask("name"))).
			Join(db.NewQuery().
				From(&storage.DBCbsd{}).
				On(db.On(storage.CbsdTable, "id", storage.ActiveModeConfigTable, "cbsd_id")).
				Select(db.NewIncludeMask())).
			Where(sq.Eq{"cbsd_serial_number": "some_serial_number"}).
			Fetch()
		s.Require().NoError(err)

		expected = []db.Model{
			&storage.DBActiveModeConfig{},
			&storage.DBCbsdState{Name: db.MakeString("registered")},
			&storage.DBCbsd{},
		}
		s.Assert().Equal(expected, actual)
	})
	s.Require().NoError(err)
}

func (s *CbsdManagerTestSuite) TestCreateCbsdWithExistingSerialNumber() {
	err := s.cbsdManager.CreateCbsd(someNetwork, getBaseCbsd())
	s.Require().NoError(err)
	err = s.cbsdManager.CreateCbsd(someNetwork, getBaseCbsd())
	s.Assert().ErrorIs(err, merrors.ErrAlreadyExists)
}

func (s *CbsdManagerTestSuite) TestUpdateCbsdWithSerialNumberOfExistingCbsd() {
	cbsd1 := getBaseCbsd()
	cbsd1.Id = db.MakeInt(1)
	cbsd2 := getBaseCbsd()
	cbsd2.Id = db.MakeInt(2)
	cbsd2.CbsdSerialNumber = db.MakeString("cbsd_serial_number2")
	err := s.cbsdManager.CreateCbsd(someNetwork, cbsd1)
	s.Require().NoError(err)
	err = s.cbsdManager.CreateCbsd(someNetwork, cbsd2)
	s.Require().NoError(err)

	cbsd2.CbsdSerialNumber = cbsd1.CbsdSerialNumber

	err = s.cbsdManager.UpdateCbsd(someNetwork, cbsd2.Id.Int64, cbsd2)
	s.Assert().ErrorIs(err, merrors.ErrAlreadyExists)
}

func (s *CbsdManagerTestSuite) TestUpdateCbsd() {
	var cbsdId int64
	err := s.resourceManager.InTransaction(func() {
		state := s.enumMaps[storage.CbsdStateTable]["registered"]
		cbsdId = s.givenResourceInserted(getCbsd(someNetwork, state))
	})
	s.Require().NoError(err)

	cbsd := getBaseCbsd()
	cbsd.UserId.String += "new1"
	cbsd.FccId.String += "new2"
	cbsd.CbsdSerialNumber.String += "new3"
	cbsd.AntennaGain.Float64 += 1
	cbsd.MaxPower.Float64 += 2
	cbsd.MinPower.Float64 += 3
	cbsd.NumberOfPorts.Int64 += 4
	err = s.cbsdManager.UpdateCbsd(someNetwork, cbsdId, cbsd)
	s.Require().NoError(err)

	err = s.resourceManager.InTransaction(func() {
		actual, err := db.NewQuery().
			WithBuilder(s.resourceManager.GetBuilder()).
			From(&storage.DBCbsd{}).
			Select(db.NewExcludeMask("id", "state_id", "cbsd_id", "grant_attempts", "is_deleted")).
			Where(sq.Eq{"id": cbsdId}).
			Fetch()
		s.Require().NoError(err)
		cbsd.NetworkId = db.MakeString(someNetwork)
		cbsd.ShouldDeregister = db.MakeBool(true)
		expected := []db.Model{cbsd}
		s.Assert().Equal(expected, actual)
	})
	s.Require().NoError(err)
}

func (s *CbsdManagerTestSuite) TestUpdateDeletedCbsd() {
	cbsdId := s.givenDeletedCbsd()

	err := s.cbsdManager.UpdateCbsd(someNetwork, cbsdId, getBaseCbsd())
	s.Assert().ErrorIs(err, merrors.ErrNotFound)
}

func (s *CbsdManagerTestSuite) TestUpdateNonExistentCbsd() {
	err := s.cbsdManager.UpdateCbsd(someNetwork, 0, getBaseCbsd())
	s.Assert().ErrorIs(err, merrors.ErrNotFound)
}

func (s *CbsdManagerTestSuite) TestDeleteCbsd() {
	var cbsdId int64
	err := s.resourceManager.InTransaction(func() {
		state := s.enumMaps[storage.CbsdStateTable]["registered"]
		cbsdId = s.givenResourceInserted(getCbsd(someNetwork, state))
	})
	s.Require().NoError(err)

	err = s.cbsdManager.DeleteCbsd(someNetwork, cbsdId)
	s.Require().NoError(err)

	err = s.resourceManager.InTransaction(func() {
		actual, err := db.NewQuery().
			WithBuilder(s.resourceManager.GetBuilder()).
			From(&storage.DBCbsd{}).
			Select(db.NewIncludeMask("is_deleted")).
			Where(sq.Eq{"id": cbsdId}).
			Fetch()
		s.Require().NoError(err)
		expected := []db.Model{
			&storage.DBCbsd{IsDeleted: db.MakeBool(true)},
		}
		s.Assert().Equal(expected, actual)
	})
	s.Require().NoError(err)
}

func (s *CbsdManagerTestSuite) TestDeletedDeletedCbsd() {
	cbsdId := s.givenDeletedCbsd()

	err := s.cbsdManager.DeleteCbsd(someNetwork, cbsdId)
	s.Assert().ErrorIs(err, merrors.ErrNotFound)
}

func (s *CbsdManagerTestSuite) TestDeleteNonExistentCbsd() {
	err := s.cbsdManager.DeleteCbsd(someNetwork, 0)
	s.Assert().ErrorIs(err, merrors.ErrNotFound)
}

func (s *CbsdManagerTestSuite) TestFetchDeletedCbsd() {
	cbsdId := s.givenDeletedCbsd()

	_, err := s.cbsdManager.FetchCbsd(someNetwork, cbsdId)
	s.Assert().ErrorIs(err, merrors.ErrNotFound)
}

func (s *CbsdManagerTestSuite) TestFetchCbsdFromDifferentNetwork() {
	var cbsdId int64
	err := s.resourceManager.InTransaction(func() {
		state := s.enumMaps[storage.CbsdStateTable]["registered"]
		cbsdId = s.givenResourceInserted(getCbsd(otherNetwork, state))
	})
	s.Require().NoError(err)

	_, err = s.cbsdManager.FetchCbsd(someNetwork, cbsdId)

	s.Assert().ErrorIs(err, merrors.ErrNotFound)
}

func (s *CbsdManagerTestSuite) TestFetchCbsdWithoutGrant() {
	var cbsdId int64
	err := s.resourceManager.InTransaction(func() {
		state := s.enumMaps[storage.CbsdStateTable]["registered"]
		cbsdId = s.givenResourceInserted(getCbsd(someNetwork, state))
	})
	s.Require().NoError(err)
	actual, err := s.cbsdManager.FetchCbsd(someNetwork, cbsdId)
	s.Require().NoError(err)

	expected := &storage.DetailedCbsd{
		Cbsd:       getDetailedCbsd(cbsdId),
		CbsdState:  &storage.DBCbsdState{Name: db.MakeString("registered")},
		Grant:      &storage.DBGrant{},
		GrantState: &storage.DBGrantState{},
	}
	s.Assert().Equal(expected, actual)
}

func (s *CbsdManagerTestSuite) TestFetchCbsdWithIdleGrant() {
	var cbsdId int64
	err := s.resourceManager.InTransaction(func() {
		state := s.enumMaps[storage.CbsdStateTable]["registered"]
		cbsdId = s.givenResourceInserted(getCbsd(someNetwork, state))
		grantState := s.enumMaps[storage.GrantStateTable]["idle"]
		s.givenResourceInserted(getGrant(grantState, cbsdId))
	})
	s.Require().NoError(err)

	actual, err := s.cbsdManager.FetchCbsd(someNetwork, cbsdId)
	s.Require().NoError(err)

	expected := &storage.DetailedCbsd{
		Cbsd:       getDetailedCbsd(cbsdId),
		CbsdState:  &storage.DBCbsdState{Name: db.MakeString("registered")},
		Grant:      &storage.DBGrant{},
		GrantState: &storage.DBGrantState{},
	}
	s.Assert().Equal(expected, actual)
}

func (s *CbsdManagerTestSuite) TestFetchCbsdWithGrant() {
	var cbsdId int64
	err := s.resourceManager.InTransaction(func() {
		state := s.enumMaps[storage.CbsdStateTable]["registered"]
		cbsdId = s.givenResourceInserted(getCbsd(someNetwork, state))
		grantState := s.enumMaps[storage.GrantStateTable]["authorized"]
		s.givenResourceInserted(getGrant(grantState, cbsdId))
	})
	s.Require().NoError(err)

	actual, err := s.cbsdManager.FetchCbsd(someNetwork, cbsdId)
	s.Require().NoError(err)

	expected := &storage.DetailedCbsd{
		Cbsd:       getDetailedCbsd(cbsdId),
		CbsdState:  &storage.DBCbsdState{Name: db.MakeString("registered")},
		Grant:      getBaseGrant(),
		GrantState: &storage.DBGrantState{Name: db.MakeString("authorized")},
	}
	s.Assert().Equal(expected, actual)
}

func (s *CbsdManagerTestSuite) TestListCbsdFromDifferentNetwork() {
	err := s.resourceManager.InTransaction(func() {
		state := s.enumMaps[storage.CbsdStateTable]["registered"]
		s.givenResourceInserted(getCbsd(otherNetwork, state))
	})
	s.Require().NoError(err)

	actual, err := s.cbsdManager.ListCbsd(someNetwork, &storage.Pagination{}, nil)
	s.Require().NoError(err)

	expected := &storage.DetailedCbsdList{
		Cbsds: []*storage.DetailedCbsd{},
		Count: 0,
	}
	s.Assert().Equal(expected, actual)
}

func (s *CbsdManagerTestSuite) TestListWithPagination() {
	const count = 4
	models := make([]db.Model, count)
	stateId := s.enumMaps[storage.CbsdStateTable]["unregistered"]
	for i := range models {
		cbsd := getCbsd(someNetwork, stateId)
		cbsd.Id = db.MakeInt(int64(i + 1))

		cbsd.CbsdSerialNumber = db.MakeString(fmt.Sprintf("some_serial_number%d", i+1))
		models[i] = cbsd
	}
	err := s.resourceManager.InsertResources(db.NewExcludeMask(), models...)
	s.Require().NoError(err)

	const limit = 2
	const offset = 1
	pagination := &storage.Pagination{
		Limit:  db.MakeInt(limit),
		Offset: db.MakeInt(offset),
	}
	actual, err := s.cbsdManager.ListCbsd(someNetwork, pagination, nil)
	s.Require().NoError(err)

	expected := &storage.DetailedCbsdList{
		Count: count,
		Cbsds: make([]*storage.DetailedCbsd, limit),
	}
	for i := range expected.Cbsds {
		cbsd := getDetailedCbsd(int64(i + 1 + offset))
		cbsd.CbsdSerialNumber = db.MakeString(fmt.Sprintf("some_serial_number%d", i+1+offset))
		expected.Cbsds[i] = &storage.DetailedCbsd{
			Cbsd:       cbsd,
			CbsdState:  &storage.DBCbsdState{Name: db.MakeString("unregistered")},
			Grant:      &storage.DBGrant{},
			GrantState: &storage.DBGrantState{},
		}
	}
	s.Assert().Equal(expected, actual)
}

func (s *CbsdManagerTestSuite) TestListWithFilter() {
	const count = 2
	models := make([]db.Model, count)
	stateId := s.enumMaps[storage.CbsdStateTable]["unregistered"]
	for i := range models {
		cbsd := getCbsd(someNetwork, stateId)
		cbsd.Id = db.MakeInt(int64(i + 1))

		cbsd.CbsdSerialNumber = db.MakeString(fmt.Sprintf("some_serial_number%d", i+1))
		models[i] = cbsd
	}
	err := s.resourceManager.InsertResources(db.NewExcludeMask(), models...)
	s.Require().NoError(err)

	pagination := &storage.Pagination{}
	filter := &storage.CbsdFilter{SerialNumber: "some_serial_number1"}
	actual, err := s.cbsdManager.ListCbsd(someNetwork, pagination, filter)
	s.Require().NoError(err)

	expected := &storage.DetailedCbsdList{
		Count: count,
		Cbsds: make([]*storage.DetailedCbsd, 1),
	}
	for i := range expected.Cbsds {
		cbsd := getDetailedCbsd(int64(i + 1))
		cbsd.CbsdSerialNumber = db.MakeString(fmt.Sprintf("some_serial_number%d", i+1))
		expected.Cbsds[i] = &storage.DetailedCbsd{
			Cbsd:       cbsd,
			CbsdState:  &storage.DBCbsdState{Name: db.MakeString("unregistered")},
			Grant:      &storage.DBGrant{},
			GrantState: &storage.DBGrantState{},
		}
	}
	s.Assert().Equal(expected, actual)
}

func (s *CbsdManagerTestSuite) TestListNotIncludeIdleGrants() {
	var cbsdId int64
	err := s.resourceManager.InTransaction(func() {
		state := s.enumMaps[storage.CbsdStateTable]["registered"]
		cbsdId = s.givenResourceInserted(getCbsd(someNetwork, state))
		grantState := s.enumMaps[storage.GrantStateTable]["idle"]
		s.givenResourceInserted(getGrant(grantState, cbsdId))
		s.givenResourceInserted(getGrant(grantState, cbsdId))
	})
	s.Require().NoError(err)

	actual, err := s.cbsdManager.ListCbsd(someNetwork, &storage.Pagination{}, nil)
	s.Require().NoError(err)

	expected := &storage.DetailedCbsdList{
		Cbsds: []*storage.DetailedCbsd{{
			Cbsd:       getDetailedCbsd(cbsdId),
			CbsdState:  &storage.DBCbsdState{Name: db.MakeString("registered")},
			Grant:      &storage.DBGrant{},
			GrantState: &storage.DBGrantState{},
		}},
		Count: 1,
	}
	s.Assert().Equal(expected, actual)
}

func (s *CbsdManagerTestSuite) TestListDeletedCbsd() {
	s.givenDeletedCbsd()

	actual, err := s.cbsdManager.ListCbsd(someNetwork, &storage.Pagination{}, nil)
	s.Require().NoError(err)

	expected := &storage.DetailedCbsdList{
		Cbsds: []*storage.DetailedCbsd{},
		Count: 0,
	}
	s.Assert().Equal(expected, actual)
}

func (s *CbsdManagerTestSuite) givenResourceInserted(model db.Model) int64 {
	id, err := db.NewQuery().
		WithBuilder(s.resourceManager.GetBuilder()).
		From(model).
		Select(db.NewExcludeMask("id")).
		Insert()
	s.Require().NoError(err)
	return id
}

func (s *CbsdManagerTestSuite) givenDeletedCbsd() int64 {
	var cbsdId int64
	err := s.resourceManager.InTransaction(func() {
		state := s.enumMaps[storage.CbsdStateTable]["registered"]
		cbsd := getCbsd(someNetwork, state)
		cbsd.IsDeleted = db.MakeBool(true)
		cbsdId = s.givenResourceInserted(cbsd)
	})
	s.Require().NoError(err)
	return cbsdId
}

func (s *CbsdManagerTestSuite) getNameIdMapping(model db.Model) map[string]int64 {
	var resources [][]db.Model
	err := s.resourceManager.InTransaction(func() {
		var err error
		resources, err = db.NewQuery().
			WithBuilder(s.resourceManager.GetBuilder()).
			From(model).
			Select(db.NewExcludeMask()).
			List()
		s.Require().NoError(err)
	})
	s.Require().NoError(err)
	m := make(map[string]int64, len(resources))
	for _, r := range resources {
		enum := r[0].(storage.EnumModel)
		m[enum.GetName()] = enum.GetId()
	}
	return m
}

func getBaseGrant() *storage.DBGrant {
	base := &storage.DBGrant{}
	base.GrantExpireTime = db.MakeTime(time.Unix(123, 0).UTC())
	base.TransmitExpireTime = db.MakeTime(time.Unix(456, 0).UTC())
	base.LowFrequency = db.MakeInt(3600 * 1e6)
	base.HighFrequency = db.MakeInt(3620 * 1e6)
	base.MaxEirp = db.MakeFloat(35)
	return base
}

func getGrant(stateId int64, cbsdId int64) *storage.DBGrant {
	base := getBaseGrant()
	base.CbsdId = db.MakeInt(cbsdId)
	base.StateId = db.MakeInt(stateId)
	base.GrantId = db.MakeString("some_grant_id")
	return base
}

func getCbsd(networkId string, stateId int64) *storage.DBCbsd {
	base := getBaseCbsd()
	base.NetworkId = db.MakeString(networkId)
	base.StateId = db.MakeInt(stateId)
	base.CbsdId = db.MakeString("some_cbsd_id")
	base.ShouldDeregister = db.MakeBool(false)
	base.IsDeleted = db.MakeBool(false)
	base.GrantAttempts = db.MakeInt(0)
	return base
}

func getDetailedCbsd(id int64) *storage.DBCbsd {
	base := getBaseCbsd()
	base.Id = db.MakeInt(id)
	base.CbsdId = db.MakeString("some_cbsd_id")
	return base
}

func getBaseCbsd() *storage.DBCbsd {
	base := &storage.DBCbsd{}
	base.UserId = db.MakeString("some_user_id")
	base.FccId = db.MakeString("some_fcc_id")
	base.CbsdSerialNumber = db.MakeString("some_serial_number")
	base.PreferredBandwidthMHz = db.MakeInt(20)
	base.PreferredFrequenciesMHz = db.MakeString("[3600]")
	base.MinPower = db.MakeFloat(10)
	base.MaxPower = db.MakeFloat(20)
	base.AntennaGain = db.MakeFloat(15)
	base.NumberOfPorts = db.MakeInt(2)
	return base
}
