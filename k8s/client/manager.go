// Copyright © 2019 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package client

import (
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// New returns a new Manager with initialized client caches
func NewManager(config *rest.Config, quit <-chan struct{}, options ManagerOptions) (Manager, error) {
	if options.Scheme == nil {
		options.Scheme = GetScheme()
	}
	o := manager.Options{
		Scheme:         options.Scheme,
		MapperProvider: options.MapperProvider,
		SyncPeriod:     options.SyncPeriod,
	}
	mgr, err := manager.New(config, o)
	if err != nil {
		return nil, err
	}

	// start cache
	cache := mgr.GetCache()
	go func() {
		err = cache.Start(quit)
	}()
	cache.WaitForCacheSync(quit)

	return mgr, nil
}
