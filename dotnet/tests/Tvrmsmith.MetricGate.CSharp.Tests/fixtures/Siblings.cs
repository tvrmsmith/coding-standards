namespace Fixtures;

// Two sibling scopes declaring the same local name with the same parameters on one line. Name,
// parameter list and line range all match, so the start column in the signature is the only thing
// keeping the gate from rejecting the pair as one span reported twice.
public class Siblings
{
    public int Twice(int n)
    {
        { int L(int x) => x > 0 ? 1 : 0; n += L(n); } { int L(int x) => x; n += L(n); }

        return n;
    }
}
