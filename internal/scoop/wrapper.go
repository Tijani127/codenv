package scoop

const wrapScript = `$ErrorActionPreference = 'Continue'
$codenvUser = [System.EnvironmentVariableTarget]::User
$codenvWatched = @('PATH', 'SCOOP_PATH', 'PSModulePath')
$codenvSaved = @{}
foreach ($codenvName in $codenvWatched) {
    $codenvSaved[$codenvName] = [Environment]::GetEnvironmentVariable($codenvName, $codenvUser)
}

$codenvScoopScript = $env:CODEENV_SCOOP_SCRIPT
$codenvRaw = $env:CODEENV_SCOOP_ARGS
$codenvArgs = @()
if ($codenvRaw) {
    $codenvJson = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($codenvRaw))
    $codenvArgs = @($codenvJson | ConvertFrom-Json)
}

$codenvCode = 0
try {
    & $codenvScoopScript @codenvArgs
    if ($null -ne $LASTEXITCODE) { $codenvCode = $LASTEXITCODE }
} catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    $codenvCode = 1
} finally {
    foreach ($codenvName in $codenvWatched) {
        $codenvCurrent = [Environment]::GetEnvironmentVariable($codenvName, $codenvUser)
        if ($codenvCurrent -ne $codenvSaved[$codenvName]) {
            [Environment]::SetEnvironmentVariable($codenvName, $codenvSaved[$codenvName], $codenvUser)
        }
    }
}
exit $codenvCode
`
