const { execSync } = require('child_process');
const fs = require('fs');

console.log('=== Real E2E Test for Smara Scheduler / Cronjob ===');

try {
  // 1. Add a new schedule for "every 1m"
  console.log('\nStep 1: Adding schedule "every 1m" "test_workflow"...');
  const addOut = execSync('./smara schedule add "every 1m" "test_workflow"', { cwd: '/home/cahya/2026/Smara CLI', encoding: 'utf-8' });
  console.log('Output:\n' + addOut.trim());

  // Extract ID from output (e.g. Schedule sch-12345 dibuat...)
  const match = addOut.match(/Schedule (sch-[a-f0-9]+)/);
  if (!match) {
    throw new Error('Failed to parse schedule ID from output');
  }
  const scheduleId = match[1];
  console.log(`✓ Schedule created with ID: ${scheduleId}`);

  // 2. List schedules
  console.log('\nStep 2: Listing schedules via CLI...');
  const listOut = execSync('./smara schedule list', { cwd: '/home/cahya/2026/Smara CLI', encoding: 'utf-8' });
  console.log('Output:\n' + listOut.trim());
  if (!listOut.includes(scheduleId)) {
    throw new Error(`Schedule ID ${scheduleId} not found in list output`);
  }
  console.log('✓ Schedule verified in list output!');

  // 3. Test run-due (since NextRunAt is 1m in future, let's manually test run-due output)
  console.log('\nStep 3: Running due schedules...');
  const dueOut = execSync('./smara schedule run-due', { cwd: '/home/cahya/2026/Smara CLI', encoding: 'utf-8' });
  console.log('Output:\n' + dueOut.trim());

  // 4. Remove schedule
  console.log(`\nStep 4: Removing schedule ${scheduleId}...`);
  const removeOut = execSync(`./smara schedule remove "${scheduleId}"`, { cwd: '/home/cahya/2026/Smara CLI', encoding: 'utf-8' });
  console.log('Output:\n' + removeOut.trim());
  console.log('✓ Schedule successfully removed!');

  console.log('\n✅ Cronjob / Scheduler E2E Real Test PASSED!');
  process.exit(0);
} catch (err) {
  console.error('\n❌ Cronjob E2E Test FAILED:', err.message);
  process.exit(1);
}
