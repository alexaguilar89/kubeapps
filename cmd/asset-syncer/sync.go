/*
Copyright (c) 2018 The Helm Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"os"
	"time"

	"github.com/kubeapps/common/datastore"
	"github.com/kubeapps/kubeapps/pkg/chart/models"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync [REPO NAME] [REPO URL] [REPO TYPE]",
	Short: "add a new chart repository, and resync its charts periodically",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) != 3 {
			logrus.Info("Need exactly two arguments: [REPO NAME] [REPO URL] [REPO TYPE]")
			cmd.Help()
			return
		}

		if debug {
			logrus.SetLevel(logrus.DebugLevel)
		}

		dbConfig := datastore.Config{URL: databaseURL, Database: databaseName, Username: databaseUser, Password: databasePassword}
		kubeappsNamespace := os.Getenv("POD_NAMESPACE")
		manager, err := newManager(dbConfig, kubeappsNamespace)
		if err != nil {
			logrus.Fatal(err)
		}
		err = manager.Init()
		if err != nil {
			logrus.Fatal(err)
		}
		defer manager.Close()

		authorizationHeader := os.Getenv("AUTHORIZATION_HEADER")
		var repoIface Repo
		if args[2] == "helm" {
			repoIface, err = getHelmRepo(namespace, args[0], args[1], authorizationHeader)
		} else {
			repoIface, err = getOCIRepo(namespace, args[0], args[1], authorizationHeader, ociRepositories)
		}
		if err != nil {
			logrus.Fatal(err)
		}
		repo := repoIface.Repo()
		checksum, err := repoIface.Checksum()
		if err != nil {
			logrus.Fatal(err)
		}

		// Check if the repo has been already processed
		if manager.RepoAlreadyProcessed(models.Repo{Namespace: repo.Namespace, Name: repo.Name}, checksum) {
			logrus.WithFields(logrus.Fields{"url": repo.URL}).Info("Skipping repository since there are no updates")
			return
		}

		charts, err := repoIface.Charts()
		if err != nil {
			logrus.Fatal(err)
		}

		if err = manager.Sync(models.Repo{Name: repo.Name, Namespace: repo.Namespace}, charts); err != nil {
			logrus.Fatalf("Can't add chart repository to database: %v", err)
		}

		// Fetch and store chart icons
		fImporter := fileImporter{manager}
		fImporter.fetchFiles(charts, repoIface)

		// Update cache in the database
		if err = manager.UpdateLastCheck(repo.Namespace, repo.Name, checksum, time.Now()); err != nil {
			logrus.Fatal(err)
		}
		logrus.WithFields(logrus.Fields{"url": repo.URL}).Info("Stored repository update in cache")

		logrus.Infof("Successfully added the chart repository %s to database", args[0])
	},
}
