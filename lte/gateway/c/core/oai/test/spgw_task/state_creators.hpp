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
 */
#include <vector>

extern "C" {
#include "lte/gateway/c/core/oai/tasks/sgw/sgw_defs.h"
#include "lte/gateway/c/core/oai/include/spgw_types.h"
}

namespace magma {

gtpv1u_data_t make_gtpv1u_data(int fd0, int fd1u);

spgw_state_t make_spgw_state(uint32_t gtpv1u_teid, int fd0, int fd1u);

s_plus_p_gw_eps_bearer_context_information_t* make_bearer_context(imsi64_t imsi,
                                                                  teid_t teid);

}  // namespace magma
