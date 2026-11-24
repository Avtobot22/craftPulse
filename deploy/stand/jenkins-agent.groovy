import hudson.model.Node
import hudson.slaves.DumbSlave
import hudson.slaves.JNLPLauncher
import jenkins.model.Jenkins
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.attribute.PosixFilePermissions
def j = Jenkins.get()
j.setNumExecutors(0)
def node = j.getNode('craftpulse-ci90-agent')
if (node == null) {
    node = new DumbSlave('craftpulse-ci90-agent', '/work', new JNLPLauncher())
    node.setMode(Node.Mode.NORMAL)
    node.setNumExecutors(1)
    node.setLabelString('craftpulse-ci90')
    j.addNode(node)
}
j.save()
def target = Path.of('/agent-bootstrap/agent-secret')
Files.writeString(target, node.toComputer().getJnlpMac())
Files.setPosixFilePermissions(target, PosixFilePermissions.fromString('rw-------'))
