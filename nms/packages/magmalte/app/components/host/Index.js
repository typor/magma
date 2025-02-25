/**
 * Copyright 2020 The Magma Authors.
 *
 * This source code is licensed under the BSD-style license found in the
 * LICENSE file in the root directory of this source tree.
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * @flow strict-local
 * @format
 */

import AccountSettings from '../AccountSettings';
import AppContent from '../layout/AppContent';
import AppSideBar from '../../../fbc_js_core/ui/components/layout/AppSideBar';
import ApplicationMain from '../../components/ApplicationMain';
import AssignmentIcon from '@material-ui/icons/Assignment';
import CloudMetrics from '../../views/metrics/CloudMetrics';
import Features from '../../../fbc_js_core/ui/host/Features';
import FlagIcon from '@material-ui/icons/Flag';
import NavListItem from '../../../fbc_js_core/ui/components/NavListItem';
import OrganizationEdit from '../../../fbc_js_core/ui/host/OrganizationEdit';
import Organizations from '../../../fbc_js_core/ui/host/Organizations';
import PeopleIcon from '@material-ui/icons/People';
import React from 'react';
import ShowChartIcon from '@material-ui/icons/ShowChart';
import UsersSettings from '../admin/userManagement/UsersSettings';
import {AppContextProvider} from '../../../fbc_js_core/ui/context/AppContext';
import {Redirect, Route, Switch} from 'react-router-dom';
import {getProjectTabs as getAllProjectTabs} from '../../../fbc_js_core/projects/projects';
import {makeStyles} from '@material-ui/styles';
import {useRelativeUrl} from '../../../fbc_js_core/ui/hooks/useRouter';

const useStyles = makeStyles(() => ({
  root: {
    display: 'flex',
  },
}));

const accessibleTabs = ['NMS'];

function NavItems() {
  const relativeUrl = useRelativeUrl();
  return (
    <>
      <NavListItem
        label="Organizations"
        path={relativeUrl('/organizations')}
        icon={<AssignmentIcon />}
      />
      <NavListItem
        label="Features"
        path={relativeUrl('/features')}
        icon={<FlagIcon />}
      />
      <NavListItem
        label="Metrics"
        path={relativeUrl('/metrics')}
        icon={<ShowChartIcon />}
      />
      <NavListItem
        label="Users"
        path={relativeUrl('/users')}
        icon={<PeopleIcon />}
      />
    </>
  );
}

function Host() {
  const classes = useStyles();
  const relativeUrl = useRelativeUrl();

  return (
    <div className={classes.root}>
      <AppSideBar mainItems={<NavItems />} />
      <AppContent>
        <Switch>
          <Route
            path={relativeUrl('/organizations/detail/:name')}
            render={() => (
              <OrganizationEdit
                getProjectTabs={() =>
                  getAllProjectTabs().filter(tab =>
                    accessibleTabs.includes(tab.name),
                  )
                }
              />
            )}
          />
          <Route
            path={relativeUrl('/organizations')}
            component={Organizations}
          />
          <Route path={relativeUrl('/features')} component={Features} />
          <Route path={relativeUrl('/metrics')} component={CloudMetrics} />
          <Route path={relativeUrl('/users')} component={UsersSettings} />
          <Route path={relativeUrl('/settings')} component={AccountSettings} />
          <Redirect to={relativeUrl('/organizations')} />
        </Switch>
      </AppContent>
    </div>
  );
}

const Index = () => {
  return (
    <ApplicationMain>
      <AppContextProvider isOrganizations={true}>
        <Host />
      </AppContextProvider>
    </ApplicationMain>
  );
};

export default () => <Route path="/host" component={Index} />;
