import org.codehaus.groovy.ast.builder.AstBuilder
import org.codehaus.groovy.control.CompilePhase

// Parse only: Jenkins/plugin DSL validation still requires a Jenkins controller.
['examples/Jenkinsfile', 'examples/Jenkinsfile.docker', 'examples/Jenkinsfile.freeze'].each { path ->
    new AstBuilder().buildFromString(CompilePhase.CONVERSION, false, new File(path).text)
    println "Groovy syntax OK: ${path}"
}
