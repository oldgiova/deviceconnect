// Copyright 2022 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package mongo

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendersoftware/go-lib-micro/mongo/migrate"
	mstore "github.com/mendersoftware/go-lib-micro/store/v2"
)

type index struct {
	Keys        map[string]int `bson:"key"`
	Name        string         `bson:"name"`
	ExpireAfter *int           `bson:"expireAfterSeconds"`
}

func TestMigration_2_0_0(t *testing.T) {
	m := &migration_2_0_0{
		client: db.Client(),
		db:     DbName,
	}
	from := migrate.MakeVersion(0, 0, 0)
	db := db.Client().Database(DbName)

	collDevs := db.Client().Database(DbName).
		Collection(DevicesCollectionName)
	ctx := context.Background()

	_, err := collDevs.InsertMany(ctx, func() []interface{} {
		ret := make([]interface{}, findBatchSize+1)
		for i := range ret {
			ret[i] = bson.D{{Key: "_id", Value: strconv.Itoa(i)}}
		}
		return ret
	}())
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	err = m.Up(from)
	require.NoError(t, err)

	collNames, err := db.ListCollectionNames(ctx, bson.D{})
	for _, collName := range collNames {
		idxes, err := db.Collection(collName).
			Indexes().
			ListSpecifications(ctx)
		require.NoError(t, err)
		expectedLen := 1
		switch collName {
		case RecordingsCollectionName:
			expectedLen = 3
		case SessionsCollectionName, ControlCollectionName:
			expectedLen = 2
		}
		require.Len(t, idxes, expectedLen)
		for _, idx := range idxes {
			if idx.Name == "_id_" {
				// Skip default index
				continue
			}
			switch idx.Name {
			case IndexNameLogsExpire:
				var keys bson.D
				assert.True(t, idx.ExpireAfterSeconds != nil, "Expected index to have TTL")
				err := bson.Unmarshal(idx.KeysDocument, &keys)
				require.NoError(t, err)
				assert.Equal(t, bson.D{
					{Key: "expire_ts", Value: int32(1)},
				}, keys)

			case mstore.FieldTenantID + "_" + dbFieldSessionID:
				var keys bson.D
				err := bson.Unmarshal(idx.KeysDocument, &keys)
				require.NoError(t, err)
				assert.Equal(t, bson.D{
					{Key: mstore.FieldTenantID, Value: int32(1)},
					{Key: dbFieldSessionID, Value: int32(1)},
				}, keys)

			case mstore.FieldTenantID + "_" + dbFieldDeviceID:
				var keys bson.D
				err := bson.Unmarshal(idx.KeysDocument, &keys)
				require.NoError(t, err)
				assert.Equal(t, bson.D{
					{Key: mstore.FieldTenantID, Value: int32(1)},
					{Key: dbFieldDeviceID, Value: int32(1)},
				}, keys)

			default:
				assert.Failf(t, "index test failed", "Index name \"%s\" not recognized", idx.Name)
			}
		}
	}
	assert.Equal(t, "2.0.0", m.Version().String())

	actual, err := collDevs.CountDocuments(ctx, bson.D{
		{Key: mstore.FieldTenantID, Value: bson.D{{Key: "$exists", Value: true}}},
	})
	if assert.NoError(t, err) {
		expected, err := collDevs.CountDocuments(ctx, bson.D{})
		if assert.NoError(t, err) {
			assert.Equalf(t, expected, actual,
				"%d documents does not contain 'tenant_id' key", expected-actual)
		}
	}
}
