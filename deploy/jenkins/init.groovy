import groovy.json.JsonOutput
import hudson.model.*
import hudson.security.*
import jenkins.model.Jenkins
import jenkins.model.JenkinsLocationConfiguration
import jenkins.security.ApiTokenProperty
import org.jenkinsci.plugins.matrixauth.AuthorizationType
import org.jenkinsci.plugins.matrixauth.PermissionEntry
import org.jenkinsci.plugins.workflow.job.WorkflowJob
import org.jenkinsci.plugins.workflow.cps.CpsFlowDefinition

def j = Jenkins.get()
// Bootstrap once. Restoring a volume must preserve its tokens and job history.
if (new File(j.rootDir, '.craftpulse90-initialized').exists()) return
def secretDir = new File('/bootstrap')
def realm = new HudsonPrivateSecurityRealm(false)
j.setSecurityRealm(realm)
def credentials = [:]
['cp90-admin', 'cp90-reader'].each { name ->
    def password = new File(secretDir, name + '.password').text.trim()
    def user = realm.createAccount(name, password)
    def token = user.getProperty(ApiTokenProperty.class).tokenStore.generateNewToken('stand90')
    user.save()
    credentials[name] = [password: password, token: token.plainValue]
}
def strategy = new ProjectMatrixAuthorizationStrategy()
strategy.add(Jenkins.ADMINISTER, new PermissionEntry(AuthorizationType.USER, 'cp90-admin'))
strategy.add(Jenkins.READ, new PermissionEntry(AuthorizationType.USER, 'cp90-reader'))
j.setAuthorizationStrategy(strategy)
j.setNumExecutors(1)
j.setSlaveAgentPort(-1)
j.setCrumbIssuer(new hudson.security.csrf.DefaultCrumbIssuer(true))
JenkinsLocationConfiguration.get().setUrl('https://jenkins.defects.team/')
['org-a', 'org-b', 'control'].each { org ->
    def job = j.getItem('cp90-' + org) ?: j.createProject(WorkflowJob.class, 'cp90-' + org)
    job.setDefinition(new CpsFlowDefinition('''node {
  stage('Infrastructure smoke') {
    echo 'craftPulse test source is ready. Demo dataset is provisioned separately.'
  }
}''', true))
    if (org != 'control') {
        def acl = new AuthorizationMatrixProperty()
        acl.add(Item.READ, new PermissionEntry(AuthorizationType.USER, 'cp90-reader'))
        job.addProperty(acl)
    }
    job.save()
}
j.save()
def output = new File(secretDir, 'credentials.json')
output.text = JsonOutput.prettyPrint(JsonOutput.toJson(credentials)) + '\n'
output.setReadable(false, false)
output.setWritable(false, false)
output.setReadable(true, true)
output.setWritable(true, true)
new File(j.rootDir, '.craftpulse90-initialized').text = '1\n'
