// Copyright 2023 The Armored Witness Applet authors. All Rights Reserved.
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

package update_test

//go:generate mockgen -write_package_comment=false -self_package github.com/transparency-dev/armored-witness-applet/trusted_applet/internal/update_test -package update_test -destination mock_update_test.go  github.com/transparency-dev/armored-witness-applet/trusted_applet/internal/update Local,Remote,FirmwareVerifier

import (
	"context"
	"testing"

	"github.com/coreos/go-semver/semver"
	"github.com/golang/mock/gomock"
	"github.com/transparency-dev/armored-witness-applet/trusted_applet/internal/firmware"
	"github.com/transparency-dev/armored-witness-applet/trusted_applet/internal/update"
)

func TestUpdate(t *testing.T) {
	testCases := []struct {
		desc           string
		localApp       semver.Version
		localOS        semver.Version
		remoteApp      semver.Version
		remoteOS       semver.Version
		wantOSInstall  bool
		wantAppInstall bool
	}{
		{
			desc:           "No changes",
			localOS:        *semver.New("1.0.1"),
			localApp:       *semver.New("1.0.2"),
			remoteOS:       *semver.New("1.0.1"),
			remoteApp:      *semver.New("1.0.2"),
			wantOSInstall:  false,
			wantAppInstall: false,
		},
		{
			desc:           "OS update",
			localOS:        *semver.New("1.0.1"),
			localApp:       *semver.New("1.0.2"),
			remoteOS:       *semver.New("1.0.3"),
			remoteApp:      *semver.New("1.0.2"),
			wantOSInstall:  true,
			wantAppInstall: false,
		},
		{
			desc:           "Applet update",
			localOS:        *semver.New("1.0.1"),
			localApp:       *semver.New("1.0.2"),
			remoteOS:       *semver.New("1.0.1"),
			remoteApp:      *semver.New("1.0.4"),
			wantOSInstall:  false,
			wantAppInstall: true,
		},
		{
			desc:           "Both update",
			localOS:        *semver.New("1.0.1"),
			localApp:       *semver.New("1.0.2"),
			remoteOS:       *semver.New("1.0.3"),
			remoteApp:      *semver.New("1.0.4"),
			wantOSInstall:  true,
			wantAppInstall: true, // In reality this won't happen because OS install will cause reboot
		},
		{
			desc:           "Downgrade",
			localOS:        *semver.New("1.0.3"),
			localApp:       *semver.New("1.0.4"),
			remoteOS:       *semver.New("1.0.1"),
			remoteApp:      *semver.New("1.0.2"),
			wantOSInstall:  false,
			wantAppInstall: false,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			local := NewMockLocal(ctrl)
			remote := NewMockRemote(ctrl)
			verifier := NewMockFirmwareVerifier(ctrl)

			local.EXPECT().GetInstalledVersions().Return(tC.localOS, tC.localApp, nil)

			updater, err := update.NewUpdater(local, remote, verifier)
			if err != nil {
				t.Fatalf("NewUpdater(): %v", err)
			}

			ctx := context.Background()
			remote.EXPECT().GetLatestVersions().Return(tC.remoteOS, tC.remoteApp, nil)
			if tC.wantOSInstall {
				osDownload := firmware.Bundle{}
				remote.EXPECT().GetOS().Return(osDownload, nil)
				verifier.EXPECT().Verify(gomock.Eq(osDownload)).Return(nil)
				local.EXPECT().InstallOS(gomock.Eq(osDownload)).Return(nil)
			}
			if tC.wantAppInstall {
				appDownload := firmware.Bundle{}
				remote.EXPECT().GetApplet().Return(appDownload, nil)
				verifier.EXPECT().Verify(gomock.Eq(appDownload)).Return(nil)
				local.EXPECT().InstallApplet(gomock.Eq(appDownload)).Return(nil)
			}

			if err := updater.Update(ctx); err != nil {
				t.Errorf("Update(): %v", err)
			}
		})
	}
}
